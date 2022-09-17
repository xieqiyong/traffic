package output

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"perfma-replay/listener"

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

// NewFileResponseOutput constructor for FileOutput, accepts path
func NewFileResponseOutput(pathTemplate string, config *FileOutputConfig) *FileResponseOutput {
	o := new(FileResponseOutput)
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

func (o *FileResponseOutput) filename() string {
	defer o.mu.Unlock()
	o.mu.Lock()

	path := o.pathTemplate

	for name, fn := range dateFileResponseNameFuncs {
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

func (o *FileResponseOutput) updateName() {
	o.currentName = filepath.Clean(o.filename())
}


func (o *FileResponseOutput) PluginWriter(msg *message.OutPutMessage) (n int, err error) {
	if o.requestPerFile {
		meta := proto.PayloadMeta(msg.Meta)
		o.payloadType = meta[0]
		o.currentID = meta[1]
	}
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
		if err != nil {
			log.Fatal(o, "Cannot open file %q. Error: %s", o.currentName, err)
		}
		o.queueLength = 0
		o.mu.Unlock()
	}
	// 组装数据
	content, flag := AssembleResponse(msg)
	if flag == false {
		return 0, nil
	}
	o.writer.Write(content)
	o.writer.Write([]byte("\r\n"))

	o.queueLength++
	return len(content), nil
}

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
			noData := handlerMessage.AssembleHttpResponseData(msg, currentID)
			content, _ := json.Marshal(handlerMessage)
			return content, noData
		}
	}
	return nil, false
}



func (o *FileResponseOutput) flush() {
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

func (o *FileResponseOutput) String() string {
	return "File output: " + o.file.Name()
}

func (o *FileResponseOutput) Close() error {
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
