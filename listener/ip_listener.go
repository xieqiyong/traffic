package listener

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"io"
	"log"
	"net"
	"os"
	"perfma-replay/core"
	"perfma-replay/proto"
	"perfma-replay/size"
	"perfma-replay/tcp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ipPacket struct {
	srcIP     []byte
	dstIP     []byte
	payload   []byte
	timestamp time.Time
	newPacket core.NewPacket
}

type PcapOptions struct {
	BufferTimeout time.Duration `json:"input-raw-buffer-timeout"`
	TimestampType string        `json:"input-raw-timestamp-type"`
	BPFFilter     string        `json:"input-raw-bpf-filter"`
	BufferSize    size.Size     `json:"input-raw-buffer-size"`
	Promiscuous   bool          `json:"input-raw-promisc"`
	Monitor       bool          `json:"input-raw-monitor"`
	Snaplen       bool          `json:"input-raw-override-snaplen"`
}

const (
	Dubbo = "dubbo"
	Http  = "http"
)

var stats *expvar.Map

func init() {
	stats = expvar.NewMap("raw")
	stats.Init()
}

// 协议类型
var BizProtocolType string

type IPListener struct {
	sync.Mutex
	// IP to listen
	addr string
	// Port to listen
	ports           []uint16
	trackResponse   bool
	Interfaces      []pcap.Interface
	ipPacketsChan   chan *ipPacket
	readyChan       chan bool
	transport       string
	protocol        tcp.TCPProtocol
	messages        chan *tcp.Message
	Transport       string
	host            string
	Reading         chan bool
	Handles         map[string]packetHandle
	closeDone       chan struct{}
	quit            chan struct{}
	expiry          time.Duration
	allowIncomplete bool
	loopIndex       int
	Activate        func() error
	PcapOptions
}

type packetHandle struct {
	handler gopacket.PacketDataSource
	ips     []net.IP
}

type PcapStatProvider interface {
	Stats() (*pcap.Stats, error)
}

func NewIPListener(host string, ports []uint16, trackResponse bool, expiry time.Duration, allowIncomplete bool) (l *IPListener, err error) {
	l = &IPListener{}
	l.host = host
	if l.host == "localhost" {
		l.host = "127.0.0.1"
	}
	l.Handles = make(map[string]packetHandle)
	l.closeDone = make(chan struct{})
	l.quit = make(chan struct{})
	l.Reading = make(chan bool)
	l.ports = ports
	l.trackResponse = trackResponse
	l.Transport = "tcp"
	l.allowIncomplete = allowIncomplete
	l.expiry = expiry
	l.messages = make(chan *tcp.Message, 10000)
	l.Activate = l.activatePcap
	err = l.setInterfaces()
	if err != nil {
		return nil, err
	}
	return
}

// DeviceNotFoundError raised if user specified wrong ip
type DeviceNotFoundError struct {
	addr string
}

func (e *DeviceNotFoundError) Error() string {
	devices, _ := pcap.FindAllDevs()

	if len(devices) == 0 {
		return "Can't get list of network interfaces, ensure that you running as root user or sudo"
	}

	var msg string
	msg += "Can't find interfaces with addr: " + e.addr + ". Provide available IP for intercepting traffic: \n"
	for _, device := range devices {
		msg += "Name: " + device.Name + "\n"
		if device.Description != "" {
			msg += "Description: " + device.Description + "\n"
		}
		for _, address := range device.Addresses {
			msg += "- IP address: " + address.IP.String() + "\n"
		}
	}

	return msg
}

func isLoopback(device pcap.Interface) bool {
	if len(device.Addresses) == 0 {
		return false
	}

	switch device.Addresses[0].IP.String() {
	case "127.0.0.1", "::1":
		return true
	}

	return false
}

func listenAllInterfaces(addr string) bool {
	switch addr {
	case "", "0.0.0.0", "[::]", "::":
		return true
	default:
		return false
	}
}

func findPcapDevices(addr string) (interfaces []pcap.Interface, err error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}

	for _, device := range devices {
		if listenAllInterfaces(addr) && len(device.Addresses) > 0 || isLoopback(device) {
			interfaces = append(interfaces, device)
			continue
		}

		for _, address := range device.Addresses {
			if device.Name == addr || address.IP.String() == addr {
				interfaces = append(interfaces, device)
				return interfaces, nil
			}
		}
	}

	if len(interfaces) == 0 {
		return nil, &DeviceNotFoundError{addr}
	}

	return interfaces, nil
}

func (l *IPListener) buildPacket(srcIP []byte, dstIP []byte, payload []byte,
	timestamp time.Time, newPacket core.NewPacket) *ipPacket {
	return &ipPacket{
		srcIP:     srcIP,
		dstIP:     dstIP,
		payload:   payload,
		timestamp: timestamp,
		newPacket: newPacket,
	}
}

func (l *IPListener) closeHandles(key string) {
	l.Lock()
	defer l.Unlock()
	if handle, ok := l.Handles[key]; ok {
		if c, ok := handle.handler.(io.Closer); ok {
			c.Close()
		}

		delete(l.Handles, key)
		if len(l.Handles) == 0 {
			close(l.closeDone)
		}
	}
}

func (l *IPListener) readPcap() {
	l.Lock()
	defer l.Unlock()
	for key, handle := range l.Handles {
		go func(key string, hndl packetHandle) {
			runtime.LockOSThread()

			defer l.closeHandles(key)
			linkSize := 14
			linkType := int(layers.LinkTypeEthernet)
			if _, ok := hndl.handler.(*pcap.Handle); ok {
				linkType = int(hndl.handler.(*pcap.Handle).LinkType())
				linkSize, ok = pcapLinkTypeLength(linkType)
				if !ok {
					if os.Getenv("GORDEBUG") != "0" {
						log.Printf("can not identify link type of an interface '%s'\n", key)
					}
					return // can't find the linktype size
				}
			}

			messageParser := tcp.NewMessageParser(l.messages, l.ports, hndl.ips, l.expiry, l.allowIncomplete)

			if l.protocol == tcp.ProtocolHTTP {
				messageParser.Start = http1StartHint
				messageParser.End = http1EndHint
			}

			timer := time.NewTicker(1 * time.Second)

			for {
				select {
				case <-l.quit:
					return
				case <-timer.C:
					if h, ok := hndl.handler.(PcapStatProvider); ok {
						s, err := h.Stats()
						if err == nil {
							stats.Add("packets_received", int64(s.PacketsReceived))
							stats.Add("packets_dropped", int64(s.PacketsDropped))
							stats.Add("packets_if_dropped", int64(s.PacketsIfDropped))
						}
					}
				default:
					data, ci, err := hndl.handler.ReadPacketData()
					log.Println("原始数据=====")
					log.Println("数据流：" + string(data))
					log.Println("结束=====")
					if err == nil {
						messageParser.PacketHandler(&tcp.PcapPacket{
							Data:     data,
							LType:    linkType,
							LTypeLen: linkSize,
							Ci:       &ci,
						})
						continue
					}
					if enext, ok := err.(pcap.NextError); ok && enext == pcap.NextErrorTimeoutExpired {
						continue
					}
					if eno, ok := err.(syscall.Errno); ok && eno.Temporary() {
						continue
					}
					if enet, ok := err.(*net.OpError); ok && (enet.Temporary() || enet.Timeout()) {
						continue
					}
					if err == io.EOF || err == io.ErrClosedPipe {
						log.Printf("stopped reading from %s interface with error %s\n", key, err)
						return
					}

					log.Printf("stopped reading from %s interface with error %s\n", key, err)
					return
				}
			}
		}(key, handle)
	}
	close(l.Reading)
}

// 录制类型、录制数据包过滤
func filterPackage(packet gopacket.Packet, data []byte) bool {
	// 是否为dubbo协议
	if Dubbo == BizProtocolType {
		icmpLayer := packet.Layer(layers.LayerTypeTCP)
		var content = icmpLayer.LayerPayload()
		// 判断字符长度
		if len(content) <= 17 {
			return false
		}
		// 转换位数
		var payload16 = fmt.Sprintf("%x", content)
		payload16 = payload16[0:4]
		// 两个字节是否为魔数 0xdabb
		if strings.Compare(payload16, "dabb") != 0 {
			return false
		}
	}
	return true
}

func (l *IPListener) IsReady() bool {
	select {
	case <-l.readyChan:
		return true
	case <-time.After(1000 * time.Second):
		return false
	}
}

func (l *IPListener) Receiver() chan *ipPacket {
	return l.ipPacketsChan
}

func pcapLinkTypeLength(lType int) (int, bool) {
	switch layers.LinkType(lType) {
	case layers.LinkTypeEthernet:
		return 14, true
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		return 4, true
	case layers.LinkTypeRaw, 12, 14:
		return 0, true
	case layers.LinkTypeIPv4, layers.LinkTypeIPv6:
		// (TODO:) look out for IP encapsulation?
		return 0, true
	case layers.LinkTypeLinuxSLL:
		return 16, true
	case layers.LinkTypeFDDI:
		return 13, true
	case 226 /*DLT_IPNET*/ :
		// https://www.tcpdump.org/linktypes/LINKTYPE_IPNET.html
		return 24, true
	default:
		return 0, false
	}
}

func http1StartHint(pckt *tcp.Packet) (isRequest, isResponse bool) {
	if proto.HasRequestTitle(pckt.Payload) {
		return true, false
	}

	if proto.HasResponseTitle(pckt.Payload) {
		return false, true
	}

	// No request or response detected
	return false, false
}

func http1EndHint(m *tcp.Message) bool {
	if m.MissingChunk() {
		return false
	}

	return proto.HasFullPayload(m, m.PacketData()...)
}

func portsFilter(transport string, direction string, ports []uint16) string {
	if len(ports) == 0 || ports[0] == 0 {
		return fmt.Sprintf("%s %s portrange 0-%d", transport, direction, 1<<16-1)
	}

	var filters []string
	for _, port := range ports {
		filters = append(filters, fmt.Sprintf("%s %s port %d", transport, direction, port))
	}
	return strings.Join(filters, " or ")
}

func hostsFilter(direction string, hosts []string) string {
	var hostsFilters []string
	for _, host := range hosts {
		hostsFilters = append(hostsFilters, fmt.Sprintf("%s host %s", direction, host))
	}

	return strings.Join(hostsFilters, " or ")
}

func interfaceAddresses(ifi pcap.Interface) []string {
	var hosts []string
	for _, addr := range ifi.Addresses {
		hosts = append(hosts, addr.IP.String())
	}
	return hosts
}

func listenAll(addr string) bool {
	switch addr {
	case "", "0.0.0.0", "[::]", "::":
		return true
	}
	return false
}

func (l *IPListener) Filter(ifi pcap.Interface) (filter string) {
	// https://www.tcpdump.org/manpages/pcap-filter.7.html

	hosts := []string{l.host}
	if listenAll(l.host) || isDevice(l.host, ifi) {
		hosts = interfaceAddresses(ifi)
	}

	filter = portsFilter(l.Transport, "dst", l.ports)

	if len(hosts) != 0 && !l.Promiscuous {
		filter = fmt.Sprintf("((%s) and (%s))", filter, hostsFilter("dst", hosts))
	} else {
		filter = fmt.Sprintf("(%s)", filter)
	}

	if l.trackResponse {
		responseFilter := portsFilter(l.Transport, "src", l.ports)

		if len(hosts) != 0 && !l.Promiscuous {
			responseFilter = fmt.Sprintf("((%s) and (%s))", responseFilter, hostsFilter("src", hosts))
		} else {
			responseFilter = fmt.Sprintf("(%s)", responseFilter)
		}

		filter = fmt.Sprintf("%s or %s", filter, responseFilter)
	}

	return
}

func (l *IPListener) Messages() chan *tcp.Message {
	return l.messages
}

func (l *IPListener) ListenBackground(ctx context.Context) chan error {
	err := make(chan error, 1)
	go func() {
		defer close(err)
		if e := l.Listen(ctx); err != nil {
			err <- e
		}
	}()
	return err
}

func (l *IPListener) Listen(ctx context.Context) (err error) {
	l.readPcap()
	done := ctx.Done()
	select {
	case <-done:
		close(l.quit) // signal close on all handles
		<-l.closeDone // wait all handles to be closed
		err = ctx.Err()
	case <-l.closeDone: // all handles closed voluntarily
	}
	return
}

func (l *IPListener) setInterfaces() (err error) {
	var pifis []pcap.Interface
	pifis, err = pcap.FindAllDevs()
	ifis, _ := net.Interfaces()
	if err != nil {
		return
	}

	for _, pi := range pifis {
		if isDevice(l.host, pi) {
			l.Interfaces = []pcap.Interface{pi}
			return
		}

		var ni net.Interface
		for _, i := range ifis {
			if i.Name == pi.Name {
				ni = i
				break
			}

			addrs, _ := i.Addrs()
			for _, a := range addrs {
				for _, pa := range pi.Addresses {
					if a.String() == pa.IP.String() {
						ni = i
						break
					}
				}
			}
		}

		if ni.Flags&net.FlagLoopback != 0 {
			l.loopIndex = ni.Index
		}

		if runtime.GOOS != "windows" {
			if len(pi.Addresses) == 0 {
				continue
			}

			if ni.Flags&net.FlagUp == 0 {
				continue
			}
		}

		l.Interfaces = append(l.Interfaces, pi)
	}
	return
}

func isDevice(addr string, ifi pcap.Interface) bool {
	// Windows npcap loopback have no IPs
	if addr == "127.0.0.1" && ifi.Name == `\Device\NPF_Loopback` {
		return true
	}

	if addr == ifi.Name {
		return true
	}

	for _, _addr := range ifi.Addresses {
		if _addr.IP.String() == addr {
			return true
		}
	}

	return false
}

func (l *IPListener) PcapHandle(ifi pcap.Interface) (handle *pcap.Handle, err error) {
	var inactive *pcap.InactiveHandle
	inactive, err = pcap.NewInactiveHandle(ifi.Name)
	if err != nil {
		return nil, fmt.Errorf("inactive handle error: %q, interface: %q", err, ifi.Name)
	}
	defer inactive.CleanUp()

	if l.TimestampType != "" && l.TimestampType != "go" {
		var ts pcap.TimestampSource
		ts, err = pcap.TimestampSourceFromString(l.TimestampType)
		fmt.Println("Setting custom Timestamp Source. Supported values: `go`, ", inactive.SupportedTimestamps())
		err = inactive.SetTimestampSource(ts)
		if err != nil {
			return nil, fmt.Errorf("%q: supported timestamps: %q, interface: %q", err, inactive.SupportedTimestamps(), ifi.Name)
		}
	}
	if l.Promiscuous {
		if err = inactive.SetPromisc(l.Promiscuous); err != nil {
			return nil, fmt.Errorf("promiscuous mode error: %q, interface: %q", err, ifi.Name)
		}
	}
	if l.Monitor {
		if err = inactive.SetRFMon(l.Monitor); err != nil && !errors.Is(err, pcap.CannotSetRFMon) {
			return nil, fmt.Errorf("monitor mode error: %q, interface: %q", err, ifi.Name)
		}
	}

	var snap int

	if !l.Snaplen {
		infs, _ := net.Interfaces()
		for _, i := range infs {
			if i.Name == ifi.Name {
				snap = i.MTU + 200
			}
		}
	}

	if snap == 0 {
		snap = 64<<10 + 200
	}

	err = inactive.SetSnapLen(snap)
	if err != nil {
		return nil, fmt.Errorf("snapshot length error: %q, interface: %q", err, ifi.Name)
	}
	if l.BufferSize > 0 {
		err = inactive.SetBufferSize(int(l.BufferSize))
		if err != nil {
			return nil, fmt.Errorf("handle buffer size error: %q, interface: %q", err, ifi.Name)
		}
	}
	if l.BufferTimeout == 0 {
		l.BufferTimeout = 2000 * time.Millisecond
	}
	err = inactive.SetTimeout(l.BufferTimeout)
	if err != nil {
		return nil, fmt.Errorf("handle buffer timeout error: %q, interface: %q", err, ifi.Name)
	}
	handle, err = inactive.Activate()
	if err != nil {
		return nil, fmt.Errorf("PCAP Activate device error: %q, interface: %q", err, ifi.Name)
	}

	bpfFilter := l.BPFFilter
	if bpfFilter == "" {
		bpfFilter = l.Filter(ifi)
	}
	fmt.Println("Interface:", ifi.Name, ". BPF Filter:", bpfFilter)
	err = handle.SetBPFFilter(bpfFilter)
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("BPF filter error: %q%s, interface: %q", err, bpfFilter, ifi.Name)
	}
	return
}

func (l *IPListener) activatePcap() error {
	var e error
	var msg string
	for _, ifi := range l.Interfaces {
		var handle *pcap.Handle
		handle, e = l.PcapHandle(ifi)
		if e != nil {
			msg += ("\n" + e.Error())
			continue
		}
		l.Handles[ifi.Name] = packetHandle{
			handler: handle,
			ips:     interfaceIPs(ifi),
		}
	}
	if len(l.Handles) == 0 {
		return fmt.Errorf("pcap handles error:%s", msg)
	}
	return nil
}

func interfaceIPs(ifi pcap.Interface) []net.IP {
	var ips []net.IP
	for _, addr := range ifi.Addresses {
		ips = append(ips, addr.IP)
	}
	return ips
}

func (l *IPListener) SetPcapOptions(opts PcapOptions) {
	l.PcapOptions = opts
}
