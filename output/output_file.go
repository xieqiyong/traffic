package output

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/proto"
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
	FlushInterval time.Duration
	SizeLimit     unitSizeVar
	QueueLimit    int
	Append        bool
}

// FileOutput output plugin
type FileOutput struct {
	mu             sync.Mutex
	pathTemplate   string
	currentName    string
	file           *os.File
	queueLength    int
	chunkSize      int
	writer         io.Writer
	requestPerFile bool
	currentID      []byte
	payloadType    []byte
	closed         bool

	config *FileOutputConfig
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

	go func() {
		for {
			time.Sleep(config.FlushInterval)
			if o.closed {
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
	defer o.mu.Unlock()
	o.mu.Lock()

	path := o.pathTemplate

	for name, fn := range dateFileNameFuncs {
		path = strings.Replace(path, name, fn(o), -1)
	}

	if !o.config.Append {
		nextChunk := false

		if o.currentName == "" ||
			((o.config.QueueLimit > 0 && o.queueLength >= o.config.QueueLimit) ||
				(o.config.SizeLimit > 0 && o.chunkSize >= int(o.config.SizeLimit))) {
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
	o.currentName = filepath.Clean(o.filename())
}

// 解析数据
func Assemble(data []byte)  []byte{
	if(listener.Dubbo == listener.BizProtocolType){
		rpcData := message.DubboOutPutFile{}
		rpcData.AssembleDubboData(data);
		_content, err := json.Marshal(rpcData)
		if err != nil {
			return  nil
		}
		return _content
	}
	if(listener.Http == listener.BizProtocolType){
		return data
	}
	return nil
}

func (o *FileOutput) Write(data []byte) (n int, err error) {
	if o.requestPerFile {
		meta := proto.PayloadMeta(data)
		o.payloadType = meta[0]
		o.currentID = meta[1]
	}
	fmt.Println(string(data))
	o.updateName()
	if o.file == nil || o.currentName != o.file.Name() {
		o.mu.Lock()
		o.Close()

		o.file, err = os.OpenFile(o.currentName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
		o.file.Sync()
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer = gzip.NewWriter(o.file)
		} else {
			o.writer = bufio.NewWriter(o.file)
		}
		w := csv.NewWriter(o.file)
		// 写入数据
		err := w.Write([]string{"url", "method", "host","cookie","body"})
		if err != nil {
			return 0, err
		}
		w.Flush()
		if err != nil {
			log.Fatal(o, "Cannot open file %q. Error: %s", o.currentName, err)
		}
		o.queueLength = 0
		o.mu.Unlock()
	}else{
		w := csv.NewWriter(o.file)
		content := Assemble(data)
		b := bufio.NewReader(bytes.NewReader(content));
		req, _ := http.ReadRequest(b)
		// 采集数据写入文件
		cookie,_ := req.Cookie("perfma-token")
		body, _ := ioutil.ReadAll(req.Body)
		collectData := []string{req.URL.String(), req.Method, req.Host, cookie.String(), fmt.Sprintf("%s",body)}
		err := w.Write(collectData)
		if err != nil {
			return 0, err
		}
		w.Flush()
	}

	o.queueLength++
	return len(data), nil
}

func checkFileIsExist(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

//func (o *FileOutput) Write1(data []byte) (n int, err error) {
//	var filename = o.pathTemplate
//	var f *os.File
//	var err1 error
//	// 打开文件
//	if checkFileIsExist(filename) {
//		f, err1 = os.OpenFile(filename, os.O_APPEND|os.O_RDWR, 0666)
//		stat, err := f.Stat()
//		if(stat.Size() >= 104857600){
//			f, err1 = os.Create(filename)
//		}
//	} else {
//		f, err1 = os.Create(filename)
//	}
//	if err != nil {
//		return 0, err
//	}
//	rpcData := message.DubboOutPutFile{}
//	rpcData.AssembleDubboData(data);
//	_content, err := json.Marshal(rpcData)
//	if err != nil {
//		return 0, err
//	}
//	content := byteutils.SliceToString(_content)
//
//	defer f.Close()
//	n, err1 = io.WriteString(f, content+"\r\n")
//	if(err1 != nil){
//		log.Println("写入文件失败")
//	}
//	return len(data), nil
//}


func (o *FileOutput) flush() {
	// Don't exit on panic
	defer func() {
		if r := recover(); r != nil {
			log.Println("PANIC while file flush: ", r, o, string(debug.Stack()))
		}
	}()

	defer o.mu.Unlock()
	o.mu.Lock()

	if o.file != nil {
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer.(*gzip.Writer).Flush()
		} else {
			o.writer.(*bufio.Writer).Flush()
		}

		if stat, err := o.file.Stat(); err == nil {
			o.chunkSize = int(stat.Size())
		}
	}
}

func (o *FileOutput) String() string {
	return "File output: " + o.file.Name()
}

func (o *FileOutput) Close() error {
	if o.file != nil {
		if strings.HasSuffix(o.currentName, ".gz") {
			o.writer.(*gzip.Writer).Close()
		} else {
			o.writer.(*bufio.Writer).Flush()
		}
		o.file.Close()
	}

	o.closed = true
	return nil
}
