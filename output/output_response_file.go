package output

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"perfma-replay/byteutils"
	"perfma-replay/listener"
	"perfma-replay/size"
	"strconv"

	"io"
	"log"
	"os"
	"path/filepath"

	"perfma-replay/message"
	"perfma-replay/proto"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

var dateFileResponseNameFuncs = map[string]func(*FileResponseOutput) string{
	"%Y":  func(o *FileResponseOutput) string { return time.Now().Format("2006") },
	"%m":  func(o *FileResponseOutput) string { return time.Now().Format("01") },
	"%d":  func(o *FileResponseOutput) string { return time.Now().Format("02") },
	"%H":  func(o *FileResponseOutput) string { return time.Now().Format("15") },
	"%M":  func(o *FileResponseOutput) string { return time.Now().Format("04") },
	"%S":  func(o *FileResponseOutput) string { return time.Now().Format("05") },
	"%NS": func(o *FileResponseOutput) string { return fmt.Sprint(time.Now().Nanosecond()) },
	"%r":  func(o *FileResponseOutput) string { return string(o.currentID) },
	"%t":  func(o *FileResponseOutput) string { return string(o.payloadType) },
}

// FileResponse output plugin
type FileResponseOutput struct {
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

// NewFileResponseOutput constructor for FileOutput, accepts path
func NewFileResponseOutput(pathTemplate string, config *FileOutputConfig) *FileResponseOutput {
	o := new(FileResponseOutput)
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

func httpResponseGetFileIndex(name string) int {
	ext := filepath.Ext(name)
	withoutExt := strings.TrimSuffix(name, ext)
	if idx := strings.LastIndex(withoutExt, "_"); idx != -1 {
		if i, err := strconv.Atoi(withoutExt[idx+1:]); err == nil {
			return i
		}
	}
	return -1
}
func httpResponseSetFileIndex(name string, idx int) string {
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
func httpResponseWithoutIndex(s string) string {
	if i := strings.LastIndex(s, "_"); i != -1 {
		return s[:i]
	}
	return s
}

type httpResponseSortByFileIndex []string

func (s httpResponseSortByFileIndex) Len() int {
	return len(s)
}
func (s httpResponseSortByFileIndex) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s httpResponseSortByFileIndex) Less(i, j int) bool {
	if httpResponseWithoutIndex(s[i]) == httpResponseWithoutIndex(s[j]) {
		return httpResponseGetFileIndex(s[i]) < httpResponseGetFileIndex(s[j])
	}
	return s[i] < s[j]
}
func (o *FileResponseOutput) filename() string {
	o.RLock()
	defer o.RUnlock()
	path := o.pathTemplate
	for name, fn := range dateFileResponseNameFuncs {
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
				return httpResponseSetFileIndex(path, 0)
			}
			sort.Sort(httpResponseSortByFileIndex(matches))
			last := matches[len(matches)-1]
			fileIndex := 0
			if idx := httpResponseGetFileIndex(last); idx != -1 {
				fileIndex = idx
				if nextChunk {
					fileIndex++
				}
			}
			return httpResponseSetFileIndex(last, fileIndex)
		}
	}
	return path
}
func (o *FileResponseOutput) updateName() {
	name := filepath.Clean(o.filename())
	o.Lock()
	o.currentName = name
	o.Unlock()
}

// 解析请求数据
func AssembleResponse(msg *message.OutPutMessage) ([]byte, bool) {
	if listener.Dubbo == listener.BizProtocolType {
		handlerMessage := message.DubboOutPutFile{}
		noData := handlerMessage.AssembleDubboData(msg)
		content, err := json.Marshal(handlerMessage)
		if err != nil {
			return nil, false
		}
		return content, noData
	}
	if listener.Http == listener.BizProtocolType {
		meta := proto.PayloadMeta(msg.Meta)
		currentID := meta[1]
		if string(meta[0]) == "2" {
			handlerMessage := message.FileHttpResponseMessage{}
			noData := handlerMessage.NewAssembleHttpResponseData(msg)
			handlerMessage.Token = string(currentID)
			//content, _ := json.Marshal(handlerMessage)
			content, _ := byteutils.JSONMarshal(handlerMessage)
			return content, noData
		}
	}
	return nil, false
}

func (o *FileResponseOutput) PluginWriter(msg *message.OutPutMessage) (n int, err error) {
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
	content, flag := AssembleResponse(msg)
	if flag == false {
		return 0, nil
	}
	n, err = o.writer.Write(content)
	o.writer.Write([]byte("\r"))
	o.currentFileSize += n
	o.QueueLength++
	return n, nil
}
func (o *FileResponseOutput) flush() {
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
func (o *FileResponseOutput) String() string {
	return "File output: " + o.file.Name()
}
func (o *FileResponseOutput) Close() error {
	o.Lock()
	defer o.Unlock()
	return o.closeLocked()
}
func (o *FileResponseOutput) IsClosed() bool {
	o.Lock()
	defer o.Unlock()
	return o.closed
}
func (o *FileResponseOutput) closeLocked() error {
	if o.file != nil {
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer.(*gzip.Writer).Close()
		} else {
			o.writer.(*bufio.Writer).Flush()
		}
		o.file.Close()
		if o.config.onResClose != nil {
			o.config.onResClose(o.file.Name())
		}
	}
	o.closed = true
	o.currentFileSize = 0
	return nil
}
