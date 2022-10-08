package input

import (
	"context"
	"errors"
	"log"
	"net"
	"perfma-replay/listener"
	"perfma-replay/message"
	"perfma-replay/proto"
	"perfma-replay/size"
	"perfma-replay/tcp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HttpInput struct {
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
	listener.PcapOptions
}

func NewHttpMessage(address string, trackResponse bool, copyBufferSize size.Size) (i *HttpInput) {
	i = new(HttpInput)
	i.quit = make(chan bool)

	host, _ports, err := net.SplitHostPort(address)
	if err != nil {
		log.Fatalf("input-raw: error while parsing address: %s", err)
	}

	var ports []uint16
	if _ports != "" {
		portsStr := strings.Split(_ports, ",")

		for _, portStr := range portsStr {
			port, err := strconv.Atoi(strings.TrimSpace(portStr))
			if err != nil {
				log.Fatalf("parsing port error: %v", err)
			}
			ports = append(ports, uint16(port))

		}
	}
	i.Host = host
	i.Ports = ports
	i.TrackResponse = trackResponse
	i.CopyBufferSize = copyBufferSize
	i.PcapOptions.Snaplen = true
	i.listen(address)
	return
}
var ErrorStopped = errors.New("reading stopped")

// 最后组装，下一步写入
func (i *HttpInput) PluginReader() (*message.OutPutMessage, error) {
	var msgTCP *tcp.Message
	var msg message.OutPutMessage
	select {
	case <-i.quit:
		return nil, ErrorStopped
	case msgTCP = <-i.listener.Messages():
		msg.Data = msgTCP.Data()
	}

	var msgType byte = proto.ResponsePayload
	if msgTCP.Direction == tcp.DirIncoming {
		msgType = proto.RequestPayload
	}
	msg.Meta = proto.PayloadHeader(msgType, msgTCP.UUID(), msgTCP.Start.UnixNano(), msgTCP.End.UnixNano()-msgTCP.Start.UnixNano())
	// to be removed....
	if msgTCP.Truncated {
		log.Println(2, "[INPUT-RAW] message truncated, increase copy-buffer-size")
	}
	// to be removed...
	if msgTCP.TimedOut {
		log.Println(2, "[INPUT-RAW] message timeout reached, increase input-raw-expire")
	}
	if i.Stats {
		stat := msgTCP.Stats
		go i.addStats(stat)
	}
	msgTCP = nil
	return &msg, nil
}



func (i *HttpInput) listen(address string) {
	log.Println("Listening for traffic on: " + address)
	i.Expire = time.Second*2
	i.listener,_ = listener.NewIPListener(i.Host, i.Ports, i.TrackResponse, i.Expire, i.AllowIncomplete)
	i.listener.SetPcapOptions(i.PcapOptions)
	err := i.listener.Activate()
	if err != nil {
		log.Fatal(err)
	}
	var ctx context.Context
	ctx, i.cancelListener = context.WithCancel(context.Background())
	errCh := i.listener.ListenBackground(ctx)
	<-i.listener.Reading
	go func() {
		<-errCh // the listener closed voluntarily
		i.Close()
	}()
}

func (i *HttpInput) Close() error {
	i.Lock()
	defer i.Unlock()
	if i.closed {
		return nil
	}
	i.cancelListener()
	close(i.quit)
	i.closed = true
	return nil
}

func (i *HttpInput) addStats(mStats tcp.Stats) {
	i.Lock()
	if len(i.messageStats) >= 10000 {
		i.messageStats = []tcp.Stats{}
	}
	i.messageStats = append(i.messageStats, mStats)
	i.Unlock()
}
