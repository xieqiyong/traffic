package message

import (
	"dubbo.apache.org/dubbo-go/v3/protocol/invocation"
	"dubbo.apache.org/dubbo-go/v3/remoting"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"log"
	"perfma-replay/proto"
	"reflect"
	"time"
)

type DubboMessage struct {
	IsIncoming bool
	Start      time.Time
	SrcPort    uint16
	DstPort    uint16
	length     uint16
	checksum   uint16
	data       []byte
}

type DubboData struct {
	Version       		string    `json:"version"`
	ClassName       	string	  `json:"className"`
	MethodName          string    `json:"methodName"`
	Interface           string	  `json:"interface"`
	Path            	string	  `json:"path"`
	Timeout				string	  `json:"timeout"`
	RequestId			string	  `json:"requestId"`
	Port                string	  `json:"port"`
	Time				string	  `json:"time"`
	SrcHost             string	  `json:"srcHost"`
	DstHost				string    `json:"dstHost"`
	Application         string	  `json:"application"`
	Arguments			[]interface{}  `json:"arguments"`
}

type DubboOutPutFile struct {
	ID  						int64  					`json:"id"`
	Version 					string  				`json:"version"`
	SerialID        			byte  					`json:"serialID"`
	TwoWay          			bool    				`json:"twoWay"`
	Event           			bool     				`json:"event"`
	Attachments        			map[string]interface{} `json:"attachments"`
	MethodName         			string         			`json:"methodName"`
	ParameterTypeNames 			[]string        		`json:"parameterTypeNames"`
	ParameterTypes  			[]reflect.Type  		`json:"parameterTypes"`
	ParameterValues 			[]reflect.Value 		`json:"parameterValues"`
	Arguments 					[]interface{} 			`json:"arguments"`
	Reply    					interface{} 			`json:"reply"`
	CallBack 					interface{} 			`json:"callBack"`
	// Refer to dubbo 2.7.6.  It is different from attachment. It is used in internal process.
	Attributes 					map[string]interface{}  `json:"attributes"`
	IsRequest					bool					`json:"isRequest,false"`
}

func (d *DubboOutPutFile)AssembleDubboData(message *OutPutMessage) bool{
	codec := proto.DubboCodec{}
	decode, _, err := codec.Decode(message.Data)
	if err != nil {
		log.Printf("解析dubbo请求数据错误:%s",err)
		return false
	}
	request := decode.Result.(*remoting.Request)
	data    := request.Data.(*invocation.RPCInvocation)
	d.Attachments  = data.Attachments()
	d.TwoWay = request.TwoWay
	d.Event  = request.Event
	d.SerialID = request.SerialID
	d.Version = request.Version
	d.ID = request.ID
	d.CallBack = data.CallBack()
	d.Attributes = data.Attributes()
	d.ParameterTypeNames = data.ParameterTypeNames()
	d.ParameterTypes = data.ParameterTypes()
	d.ParameterValues = data.ParameterValues()
	d.MethodName = data.MethodName()
	d.Reply = data.Reply()
	d.Arguments = data.Arguments()
	if(decode.IsRequest){
		d.IsRequest = true;
	}
	return true
}

func NewDubboMessage(data []byte, isIncoming bool) (m *DubboMessage) {
	m = &DubboMessage{}
	tcp := &layers.TCP{}
	err := tcp.DecodeFromBytes(data, gopacket.NilDecodeFeedback)
	if err != nil {
		log.Printf("Error decode tcp message, %v\n", err)
	}
	m.SrcPort = uint16(tcp.SrcPort)
	m.DstPort = uint16(tcp.DstPort)
	//todo : fix length
	// m.length = tcp.Length
	m.length = uint16(len(tcp.Payload))
	m.checksum = tcp.Checksum
	m.data = tcp.Payload
	m.IsIncoming = isIncoming
	return
}

func (m *DubboMessage) Data() []byte {
	return m.data
}

func (m *DubboMessage) String() string {
	return fmt.Sprintf("SrcPort: %d | DstPort: %d | Length: %d | Checksum: %d | Data: %s",
		m.SrcPort, m.DstPort, m.length, m.checksum, string(m.data))
}

func (m *DubboMessage) UUID() []byte {
	return nil
}
