package input

import (
	"log"
	"net"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/proto"
)

type HttpInput struct {
	data          chan *message.HttpMessage
	address       string
	quit          chan bool
	listener      *listener.HttpListener
	trackResponse bool
}

func NewHttpMessage(address string, trackResponse bool) (i *HttpInput) {
	i = new(HttpInput)
	i.data = make(chan *message.HttpMessage)
	i.address = address
	i.quit = make(chan bool)
	i.trackResponse = trackResponse
	i.listen(address)
	return
}

// 最后组装，下一步写入
func (i *HttpInput) PluginReader() (*message.OutPutMessage, error) {
	var outMessage message.OutPutMessage
	msg := <-i.data
	buf := msg.Data()
	newPacket := msg.NewPacket
	var header []byte
	if msg.IsIncoming {
		header = proto.PayloadHeader(proto.RequestPayload, newPacket.Ack, newPacket.Seq)
	} else {
		header = proto.PayloadHeader(proto.ResponsePayload, newPacket.Ack, newPacket.Seq)
	}
	outMessage.Meta = header
	outMessage.Data = buf
	return &outMessage, nil
}



func (i *HttpInput) listen(address string) {
	log.Println("Listening for traffic on: " + address)

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		log.Fatal("input-raw: error while parsing address", err)
	}

	i.listener = listener.NewHttpListener(host, port, i.trackResponse)

	ch := i.listener.Receiver()

	go func() {
		for {
			select {
			case <-i.quit:
				return
			default:
			}
			// Receiving TCPMessage
			m := <-ch
			i.data <- m
		}
	}()
}
