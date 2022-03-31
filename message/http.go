package message

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"log"
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
}

func NewHttpMessage(data []byte, isIncoming bool) (m *HttpMessage) {
	m = &HttpMessage{}
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

func (m *HttpMessage) Data() []byte {
	return m.data
}