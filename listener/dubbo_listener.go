package listener

import (
	"log"
	"perfma-replay/message"
	"strconv"
)

type DubboListener struct {
	// IP to listen
	addr string
	// Port to listen
	port uint16

	messagesChan chan *message.DubboMessage

	underlying *IPListener
}

func NewDubboListener(addr string, port string, trackResponse bool) (l *DubboListener) {
	l = &DubboListener{}
	l.messagesChan = make(chan *message.DubboMessage, 10000)
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

func (l *DubboListener) parseDubboPacket(packet *ipPacket) (messages *message.DubboMessage) {
	data  := packet.payload
	messages = message.NewDubboMessage(data, false)
	return
}

func (l *DubboListener) recv() {
	for {
		ipPacketsChan := l.underlying.Receiver()
		select {
		case packet := <-ipPacketsChan:
			message := l.parseDubboPacket(packet)
			l.messagesChan <- message
		}
	}
}

func (l *DubboListener) Receiver() chan *message.DubboMessage {
	return l.messagesChan
}
