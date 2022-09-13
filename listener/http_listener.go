package listener

import (
	"log"
	"perfma-replay/message"
	"strconv"
)

type HttpListener struct {
	// IP to listen
	addr string
	// Port to listen
	port uint16

	messagesChan chan *message.HttpMessage

	underlying *IPListener
}

func NewHttpListener(addr string, port string, trackResponse bool) (l *HttpListener) {
	l = &HttpListener{}
	l.messagesChan = make(chan *message.HttpMessage, 10000)
	l.addr = addr
	intPort, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("Invaild Port: %s, %v\n", port, err)
	}
	l.port = uint16(intPort)

	l.underlying = NewIPListener(addr, l.port, trackResponse)

	if l.underlying.IsReady() {
		go l.recv()
	} else {
		log.Fatalln("IP Listener is not ready after 5 seconds")
	}

	return
}

func (l *HttpListener) parseHttpPacket(packet *ipPacket) (messages *message.HttpMessage) {
	flag := false
	if packet.newPacket.DstPort == l.port {
		flag = true;
	}
	messages = message.NewHttpMessage(packet.payload, flag, packet.newPacket)
	return
}

func (l *HttpListener) recv() {
	for {
		ipPacketsChan := l.underlying.Receiver()
		select {
		case packet := <-ipPacketsChan:
			message := l.parseHttpPacket(packet)
			l.messagesChan <- message
		}
	}
}

func (l *HttpListener) Receiver() chan *message.HttpMessage {
	return l.messagesChan
}
