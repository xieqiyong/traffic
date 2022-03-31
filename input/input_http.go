package input

import (
	"log"
	"net"
	"perfma-replay/listener"
	"perfma-replay/message"
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

func (i *HttpInput) Read(data []byte) (int, error) {
	msg := <-i.data
	buf := msg.Data()

	var header []byte

	//if msg.IsIncoming {
	//	header = proto.PayloadHeader(proto.RequestPayload, msg.UUID(), msg.Start.UnixNano())
	//} else {
	//	header = proto.PayloadHeader(proto.ResponsePayload, msg.UUID(), msg.Start.UnixNano())
	//}

	copy(data[0:len(header)], header)

	copy(data[len(header):], buf)

	// copy(data, buf)

	return len(buf) + len(header), nil
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
