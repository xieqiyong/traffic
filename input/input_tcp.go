package input

import (
	"context"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/size"
	"perfma-replay/tcp"
	"sync"
	"time"
)

type TCPInput struct {
	sync.Mutex
	messageStats   []tcp.Stats
	listener       *listener.IPListener
	messageParser  *tcp.MessageParser
	cancelListener context.CancelFunc
	closed         bool
	TrackResponse  bool
	Expire          time.Duration
	CopyBufferSize  size.Size
	Stats           bool
	AllowIncomplete bool
	Host  string
	Ports []uint16
	data          chan *message.HttpMessage
	address       string
	quit          chan bool
}

func NewTCPInput(address string, trackResponse bool) (i *TCPInput) {
	i = new(TCPInput)
	//i.data = make(chan *proto.TCPMessage)
	i.address = address
	i.quit = make(chan bool)
	i.listen(address)
	return
}

func (i *TCPInput) PluginReader() (*message.OutPutMessage, error) {
	//msg := <-i.data
	//buf := msg.Data()

	//var header []byte

	//if msg.IsIncoming {
	//	header = proto.PayloadHeader(proto.RequestPayload, msg.UUID(), msg.Start.UnixNano())
	//} else {
	//	header = proto.PayloadHeader(proto.ResponsePayload, msg.UUID(), msg.Start.UnixNano())
	//}

	//copy(data[0:len(header)], header)
	//
	//copy(data[len(header):], buf)

	// copy(data, buf)
	var outMessage message.OutPutMessage
	return &outMessage, nil
}

func (i *TCPInput) listen(address string) {
	//log.Println("Listening for traffic on: " + address)
	//
	//host, port, err := net.SplitHostPort(address)
	//
	//if err != nil {
	//	log.Fatal("input-raw: error while parsing address", err)
	//}
	//
	//i.listener = listener.NewTCPListener(host, port, i.trackResponse)
	//
	//ch := i.listener.Receiver()
	//
	//go func() {
	//	for {
	//		select {
	//		case <-i.quit:
	//			return
	//		default:
	//		}
	//		// Receiving TCPMessage
	//		m := <-ch
	//		i.data <- m
	//	}
	//}()
}
