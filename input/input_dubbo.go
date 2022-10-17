package input

import (
	"context"
	"log"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/size"
	"perfma-replay/tcp"
	"sync"
	"time"
)

type DubboInput struct {
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
	address       string
	quit          chan bool
}

func NewDubboMessage(address string, trackResponse bool) (i *DubboInput) {
	i = new(DubboInput)
	//i.data = make(chan *message.DubboMessage)
	i.address = address
	i.quit = make(chan bool)
	//i.trackResponse = trackResponse
	i.listen(address)
	return
}

func (i *DubboInput) PluginReader() (*message.OutPutMessage, error) {
	//msg := <-i.data
	//buf := msg.Data()
	var outMessage message.OutPutMessage

	//if msg.IsIncoming {
	//	header = proto.PayloadHeader(proto.RequestPayload, msg.UUID(), msg.Start.UnixNano())
	//} else {
	//	header = proto.PayloadHeader(proto.ResponsePayload, msg.UUID(), msg.Start.UnixNano())
	//}



	// copy(data, buf)

	return &outMessage, nil
}



func (i *DubboInput) listen(address string) {
	log.Println("Listening for traffic on: " + address)

	//host, port, err := net.SplitHostPort(address)

	//if err != nil {
	//	log.Fatal("input-raw: error while parsing address", err)
	//}

	//i.listener = listener.NewDubboListener(host, port, i.trackResponse)
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
