package output

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/proto"
	"perfma-replay/size"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type unitSizeVar int64

var dataUnitMap = map[byte]int64{
	'k': 1024,
	'm': 1024 * 1024,
	'g': 1024 * 1024 * 1024,
}
var payloadSeparatorAsBytes = []byte(proto.PayloadSeparator)

func parseDataUnit(s string) int64 {
	// Allow kb, mb, gb
	if strings.HasSuffix(s, "b") {
		s = s[:len(s)-1]
	}
	if unit, ok := dataUnitMap[s[len(s)-1]]; ok {
		size, _ := strconv.ParseInt(s[:len(s)-1], 10, 64)
		return unit * size
	}
	// If no unit specified use bytes
	size, _ := strconv.ParseInt(s, 10, 64)
	return size
}
func (u unitSizeVar) String() string {
	return strconv.Itoa(int(u))
}
func (u *unitSizeVar) Set(s string) error {
	*u = unitSizeVar(parseDataUnit(s))
	return nil
}

var dateFileNameFuncs = map[string]func(*FileOutput) string{
	"%Y":  func(o *FileOutput) string { return time.Now().Format("2006") },
	"%m":  func(o *FileOutput) string { return time.Now().Format("01") },
	"%d":  func(o *FileOutput) string { return time.Now().Format("02") },
	"%H":  func(o *FileOutput) string { return time.Now().Format("15") },
	"%M":  func(o *FileOutput) string { return time.Now().Format("04") },
	"%S":  func(o *FileOutput) string { return time.Now().Format("05") },
	"%NS": func(o *FileOutput) string { return fmt.Sprint(time.Now().Nanosecond()) },
	"%r":  func(o *FileOutput) string { return string(o.currentID) },
	"%t":  func(o *FileOutput) string { return string(o.payloadType) },
}

type FileOutputConfig struct {
	FlushInterval time.Duration `json:"output-file-flush-interval"`
	SizeLimit     unitSizeVar   `json:"output-file-size-limit"`
	QueueLimit    int           `json:"output-file-queue-limit"`
	Append        bool          `json:"output-file-append"`
	onClose       func(string)
	onResClose    func(string)
}

// FileOutput output plugin
type FileOutput struct {
	sync.RWMutex
	pathTemplate    string
	currentName     string
	file            *os.File
	QueueLength     int
	writer          io.Writer
	requestPerFile  bool
	currentID       []byte
	payloadType     []byte
	closed          bool
	currentFileSize int
	totalFileSize   size.Size
	config          *FileOutputConfig
}

// NewFileOutput constructor for FileOutput, accepts path
func NewFileOutput(pathTemplate string, config *FileOutputConfig) *FileOutput {
	o := new(FileOutput)
	o.pathTemplate = pathTemplate
	o.config = config
	o.updateName()
	if strings.Contains(pathTemplate, "%r") {
		o.requestPerFile = true
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 100 * time.Millisecond
	}
	go func() {
		for {
			time.Sleep(config.FlushInterval)
			if o.IsClosed() {
				break
			}
			o.updateName()
			o.flush()
		}
	}()
	return o
}
func getFileIndex(name string) int {
	ext := filepath.Ext(name)
	withoutExt := strings.TrimSuffix(name, ext)
	if idx := strings.LastIndex(withoutExt, "_"); idx != -1 {
		if i, err := strconv.Atoi(withoutExt[idx+1:]); err == nil {
			return i
		}
	}
	return -1
}
func setFileIndex(name string, idx int) string {
	idxS := strconv.Itoa(idx)
	ext := filepath.Ext(name)
	withoutExt := strings.TrimSuffix(name, ext)
	if i := strings.LastIndex(withoutExt, "_"); i != -1 {
		if _, err := strconv.Atoi(withoutExt[i+1:]); err == nil {
			withoutExt = withoutExt[:i]
		}
	}
	return withoutExt + "_" + idxS + ext
}
func withoutIndex(s string) string {
	if i := strings.LastIndex(s, "_"); i != -1 {
		return s[:i]
	}
	return s
}

type sortByFileIndex []string

func (s sortByFileIndex) Len() int {
	return len(s)
}
func (s sortByFileIndex) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sortByFileIndex) Less(i, j int) bool {
	if withoutIndex(s[i]) == withoutIndex(s[j]) {
		return getFileIndex(s[i]) < getFileIndex(s[j])
	}
	return s[i] < s[j]
}
func (o *FileOutput) filename() string {
	o.RLock()
	defer o.RUnlock()
	path := o.pathTemplate
	for name, fn := range dateFileNameFuncs {
		path = strings.Replace(path, name, fn(o), -1)
	}
	if !o.config.Append {
		nextChunk := false
		if o.currentName == "" ||
			((o.config.QueueLimit > 0 && o.QueueLength >= o.config.QueueLimit) ||
				(o.config.SizeLimit > 0 && o.currentFileSize >= int(o.config.SizeLimit))) {
			nextChunk = true
		}
		ext := filepath.Ext(path)
		withoutExt := strings.TrimSuffix(path, ext)
		if matches, err := filepath.Glob(withoutExt + "*" + ext); err == nil {
			if len(matches) == 0 {
				return setFileIndex(path, 0)
			}
			sort.Sort(sortByFileIndex(matches))
			last := matches[len(matches)-1]
			fileIndex := 0
			if idx := getFileIndex(last); idx != -1 {
				fileIndex = idx
				if nextChunk {
					fileIndex++
				}
			}
			return setFileIndex(last, fileIndex)
		}
	}
	return path
}
func (o *FileOutput) updateName() {
	name := filepath.Clean(o.filename())
	o.Lock()
	o.currentName = name
	o.Unlock()
}

// 解析请求数据
func Assemble(msg *message.OutPutMessage) ([]byte, []byte, bool) {
	if listener.Dubbo == listener.BizProtocolType {
		handlerMessage := message.DubboOutPutFile{}
		noData := handlerMessage.AssembleDubboData(msg)
		content, err := json.Marshal(handlerMessage)
		if err != nil {
			return nil, nil, false
		}
		return content, nil, noData
	}
	if listener.Http == listener.BizProtocolType {
		meta := proto.PayloadMeta(msg.Meta)
		//currentID := meta[1]
		if string(meta[0]) == "1" {
			//handlerMessage := message.FileHttpRequestMessage{}
			//noData := handlerMessage.AssembleHttpRequestData(msg, currentID)
			//content, _ := byteutils.JSONMarshal(handlerMessage)
			return msg.Meta, msg.Data, true
		}
	}
	return nil, nil, false
}
func (o *FileOutput) PluginWriter(msg *message.OutPutMessage) (n int, err error) {
	if o.requestPerFile {
		o.Lock()
		meta := proto.PayloadMeta(msg.Meta)
		o.currentID = meta[1]
		o.payloadType = meta[0]
		o.Unlock()
	}
	o.updateName()
	o.Lock()
	defer o.Unlock()
	if o.file == nil || o.currentName != o.file.Name() {
		o.closeLocked()
		o.file, err = os.OpenFile(o.currentName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
		o.file.Sync()
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer = gzip.NewWriter(o.file)
		} else {
			o.writer = bufio.NewWriter(o.file)
		}
		if err != nil {
			log.Fatal(o, "Cannot open file %q. Error: %s", o.currentName, err)
		}
		o.QueueLength = 0
	}
	// 组装数据
	msgMeta, content, flag := Assemble(msg)

	if flag == false {
		return 0, nil
	}
	var nn int
	n, err = o.writer.Write(msgMeta)
	nn, err = o.writer.Write(content)
	n += nn
	nn, err = o.writer.Write(payloadSeparatorAsBytes)
	n += nn
	o.totalFileSize += size.Size(n)
	o.currentFileSize += n
	o.QueueLength++
	return n, nil
}
func (o *FileOutput) flush() {
	// Don't exit on panic
	defer func() {
		if r := recover(); r != nil {
			log.Println("PANIC while file flush: ", r, o, string(debug.Stack()))
		}
	}()
	o.Lock()
	defer o.Unlock()
	if o.file != nil {
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer.(*gzip.Writer).Flush()
		} else {
			o.writer.(*bufio.Writer).Flush()
		}
		if stat, err := o.file.Stat(); err == nil {
			o.currentFileSize = int(stat.Size())
		}
	}
}
func (o *FileOutput) String() string {
	return "File output: " + o.file.Name()
}
func (o *FileOutput) Close() error {
	o.Lock()
	defer o.Unlock()
	return o.closeLocked()
}
func (o *FileOutput) IsClosed() bool {
	o.Lock()
	defer o.Unlock()
	return o.closed
}
func (o *FileOutput) closeLocked() error {
	if o.file != nil {
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer.(*gzip.Writer).Close()
		} else {
			o.writer.(*bufio.Writer).Flush()
		}
		o.file.Close()
		if o.config.onClose != nil {
			o.config.onClose(o.file.Name())
		}
	}
	o.closed = true
	o.currentFileSize = 0
	return nil
}
