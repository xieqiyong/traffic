package message

import (
	"bufio"
	"bytes"
	"github.com/golang-module/carbon/v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/textproto"
	"net/url"
	"perfma-replay/core"
	"perfma-replay/proto"
	"strconv"
	"strings"
	"time"
)

type HttpMessage struct {
	IsIncoming bool
	Start      time.Time
	SrcPort    uint16
	DstPort    uint16
	length     uint16
	checksum   uint16
	data       []byte
	NewPacket  core.NewPacket
}

type FileHttpRequestMessage struct {
	Method      string              `json:"method"`
	Host        string              `json:"host"`
	Body        string              `json:"body"`
	Cookie      string              `json:"cookie"`
	Header      map[string][]string `json:"header"`
	Url         string              `json:"url"`
	CreateTime  string              `json:"createTime"`
	PayloadType string              `json:"payloadType"`
	Token   	string              `json:"token"`
}

type Header map[string][]string

type FileHttpResponseMessage struct {
	Status           string               `json:"Status"`     // e.g. "200 OK"
	StatusCode       int                  `json:"statusCode"` // e.g. 200
	Proto            string               `json:"proto"`      // e.g. "HTTP/1.0"
	ProtoMajor       int                  `json:"protoMajor"` // e.g. 1
	ProtoMinor       int                  `json:"protoMinor"` // e.g. 0
	Header           Header               `json:"header"`
	Body             string               `json:"body"`
	//ContentLength    int64                `json:"contentLength"`
	//TransferEncoding []string             `json:"transferEncoding"`
	//Close            bool                 `json:"close"`
	//Uncompressed     bool                 `json:"uncompressed"`
	//Trailer          Header               `json:"trailer"`
	//TLS              *tls.ConnectionState `json:"tLS"`
	PayloadType      string               `json:"payloadType"`
	CreateTime       string               `json:"createTime"`
	Token   		 string              `json:"token"`
}

func NewHttpMessage(data []byte, isIncoming bool, newPacket core.NewPacket) (m *HttpMessage) {
	m = &HttpMessage{}
	//tcp := &layers.TCP{}
	//err := tcp.DecodeFromBytes(data, gopacket.NilDecodeFeedback)
	//if err != nil {
	//	log.Printf("Error decode tcp message, %v\n", err)
	//}
	m.SrcPort = newPacket.SrcPort
	m.DstPort = newPacket.DstPort
	m.length = uint16(len(newPacket.Payload))
	m.data = newPacket.Payload
	m.IsIncoming = isIncoming
	m.NewPacket = newPacket
	return
}

func (m *HttpMessage) Data() []byte {
	return m.data
}

func (f *FileHttpRequestMessage) AssembleHttpRequestData(message *OutPutMessage, currentID []byte) bool {
	newContent := bufio.NewReader(bytes.NewReader(message.Data))
	req, _ := http.ReadRequest(newContent)
	if req == nil {
		return false
	}
	// 采集数据写入文件
	body, _ := ioutil.ReadAll(req.Body)
	f.Url = req.URL.String()
	cookie := req.Cookies()
	var cookies string
	for i := range cookie {
		cookies += cookie[i].Value
	}
	f.Header = req.Header
	f.Cookie = cookies
	f.Body, _ = (url.QueryUnescape(string(body)))
	f.Method = req.Method
	f.Host = req.Host
	f.CreateTime = carbon.Now().ToDateTimeString()
	f.PayloadType = string(proto.RequestPayload)
	f.Token = string(currentID)
	return true
}

func (f *FileHttpResponseMessage) AssembleHttpResponseData(message *OutPutMessage, currentID []byte) bool {
	newContent := bufio.NewReader(bytes.NewReader(message.Data))
	tp := textproto.NewReader(newContent)
	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return false
	}

	if i := strings.IndexByte(line, ' '); i == -1 {
		log.Print("malformed HTTP response", line)
		return false
	} else {
		f.Proto = line[:i]
		f.Status = strings.TrimLeft(line[i+1:], " ")
	}

	statusCode := f.Status
	if i := strings.IndexByte(f.Status, ' '); i != -1 {
		statusCode = f.Status[:i]
	}
	if len(statusCode) != 3 {
		log.Print("malformed HTTP status code", statusCode)
		return false
	}
	f.StatusCode, err = strconv.Atoi(statusCode)
	if err != nil || f.StatusCode < 0 {
		log.Print("malformed HTTP status code", statusCode)
		return false
	}
	var ok bool
	if f.ProtoMajor, f.ProtoMinor, ok = ParseHTTPVersion(f.Proto); !ok {
		log.Print("malformed HTTP version", f.Proto)
		return false
	}

	// Parse the response headers.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return false
	}
	f.Header = Header(mimeHeader)
	fixPragmaCacheControl(f.Header)
	nowLine := len(f.Header) + 2
	// Parse the response body
	body := ReadResponseBody(message.Data, nowLine)

	f.PayloadType = string(proto.ResponsePayload)
	f.CreateTime = carbon.Now().ToDateTimeString()
	f.Token = string(currentID)
	f.Body = body
	return true
}

func ReadResponseBody(data []byte, split int) string {
	content := bufio.NewReader(bytes.NewReader(data))
	scanner := bufio.NewScanner(content)
	var newContent string
	var i = 1
	for scanner.Scan() {
		if i > split {
			newContent += scanner.Text()
		}
		i++
	}
	return newContent
}

func fixPragmaCacheControl(header Header) {
	if hp, ok := header["Pragma"]; ok && len(hp) > 0 && hp[0] == "no-cache" {
		if _, presentcc := header["Cache-Control"]; !presentcc {
			header["Cache-Control"] = []string{"no-cache"}
		}
	}
}

func ParseHTTPVersion(vers string) (major, minor int, ok bool) {
	const Big = 1000000 // arbitrary upper bound
	switch vers {
	case "HTTP/1.1":
		return 1, 1, true
	case "HTTP/1.0":
		return 1, 0, true
	}
	if !strings.HasPrefix(vers, "HTTP/") {
		return 0, 0, false
	}
	dot := strings.Index(vers, ".")
	if dot < 0 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(vers[5:dot])
	if err != nil || major < 0 || major > Big {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(vers[dot+1:])
	if err != nil || minor < 0 || minor > Big {
		return 0, 0, false
	}
	return major, minor, true
}
