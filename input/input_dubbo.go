package input

import (
	"log"
	"net"
	"perfma-replay/listener"
	"perfma-replay/message"
)

type DubboInput struct {
	data          chan *message.DubboMessage
	address       string
	quit          chan bool
	listener      *listener.DubboListener
	trackResponse bool
}

func NewDubboMessage(address string, trackResponse bool) (i *DubboInput) {
	i = new(DubboInput)
	i.data = make(chan *message.DubboMessage)
	i.address = address
	i.quit = make(chan bool)
	i.trackResponse = trackResponse
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

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		log.Fatal("input-raw: error while parsing address", err)
	}

	i.listener = listener.NewDubboListener(host, port, i.trackResponse)

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
