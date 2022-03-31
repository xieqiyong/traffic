package listener

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ipPacket struct {
	srcIP     []byte
	dstIP     []byte
	payload   []byte
	timestamp time.Time
}

const (
	Dubbo = "dubbo"
	Http  = "http"
)

// 协议类型
var BizProtocolType string

type IPListener struct {
	mu sync.Mutex

	// IP to listen
	addr string
	// Port to listen
	port uint16

	trackResponse bool

	pcapHandles []*pcap.Handle

	ipPacketsChan chan *ipPacket

	readyChan chan bool

	transport string
}

type packetHandle struct {
	handler gopacket.PacketDataSource
	ips     []net.IP
}

func NewIPListener(addr string, port uint16, trackResponse bool) (l *IPListener) {
	l = &IPListener{}
	l.ipPacketsChan = make(chan *ipPacket, 10000)

	l.readyChan = make(chan bool, 1)
	l.addr = addr
	l.port = port
	l.trackResponse = trackResponse

	go l.readPcap()

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

func (l *IPListener) buildPacket(srcIP []byte, dstIP []byte, payload []byte, timestamp time.Time) *ipPacket {
	return &ipPacket{
		srcIP:     srcIP,
		dstIP:     dstIP,
		payload:   payload,
		timestamp: timestamp,
	}
}

func (l *IPListener) readPcap() {
	devices, err := findPcapDevices(l.addr)
	if err != nil {
		log.Fatal(err)
	}

	bpfSupported := true
	// 方便测试,线上打开
	//if runtime.GOOS == "darwin" {
	//	bpfSupported = false
	//}
	var wg sync.WaitGroup
	wg.Add(len(devices))
	for _, d := range devices {
		go func(device pcap.Interface) {
			inactive, err := pcap.NewInactiveHandle(device.Name)
			if err != nil {
				log.Println("Pcap Error while opening device", device.Name, err)
				wg.Done()
				return
			}

			if it, err := net.InterfaceByName(device.Name); err == nil {
				// Auto-guess max length of ipPacket to capture
				inactive.SetSnapLen(it.MTU + 68*2)
			} else {
				inactive.SetSnapLen(65536)
			}

			inactive.SetTimeout(-1 * time.Second)
			inactive.SetPromisc(true)

			handle, herr := inactive.Activate()
			if herr != nil {
				log.Println("PCAP Activate error:", herr)
				wg.Done()
				return
			}

			defer handle.Close()
			l.mu.Lock()
			l.pcapHandles = append(l.pcapHandles, handle)

			var bpfDstHost, bpfSrcHost string
			var loopback = isLoopback(device)

			if loopback {
				var allAddr []string
				for _, dc := range devices {
					for _, addr := range dc.Addresses {
						allAddr = append(allAddr, "(dst host "+addr.IP.String()+" and src host "+addr.IP.String()+")")
					}
				}

				bpfDstHost = strings.Join(allAddr, " or ")
				bpfSrcHost = bpfDstHost
			} else {
				for i, addr := range device.Addresses {
					bpfDstHost += "dst host " + addr.IP.String()
					bpfSrcHost += "src host " + addr.IP.String()
					if i != len(device.Addresses)-1 {
						bpfDstHost += " or "
						bpfSrcHost += " or "
					}
				}
			}

			if bpfSupported {

				var bpf string

				if l.trackResponse {
					bpf = "(tcp dst port " + strconv.Itoa(int(l.port)) + " and (" + bpfDstHost + ")) or (" + "tcp src port " + strconv.Itoa(int(l.port)) + " and (" + bpfSrcHost + "))"
				} else {
					bpf = "tcp dst port " + strconv.Itoa(int(l.port)) + " and (" + bpfDstHost + ")"
				}
				fmt.Println("Interface:", device.Name, ". BPF Filter:", bpf)
				if err := handle.SetBPFFilter(bpf); err != nil {
					log.Println("BPF filter error:", err, "Device:", device.Name, bpf)
					wg.Done()
					return
				}
			}

			// TODO: !bpfSupported

			l.mu.Unlock()

			source := gopacket.NewPacketSource(handle, handle.LinkType())
			source.Lazy = true
			source.NoCopy = true
			wg.Done()
			for {
				packet, err := source.NextPacket()
				if err == io.EOF {
					break
				} else if err != nil {
					log.Println("NextPacket error:", err)
					continue
				}
				networkLayer := packet.NetworkLayer()
				srcIP := networkLayer.NetworkFlow().Src().Raw()
				dstIP := networkLayer.NetworkFlow().Dst().Raw()
				content := networkLayer.LayerPayload();
				// 协议过滤
				isSuccess := filterPackage(packet)
				if(isSuccess == false){
					continue
				}
				// 发送数据
				l.ipPacketsChan <- l.buildPacket(srcIP, dstIP, content, packet.Metadata().Timestamp)
			}

		}(d)
	}
	wg.Wait()
	l.readyChan <- true
}

// 协议过滤
func filterPackage(packet gopacket.Packet) (bool){
	// 是否为dubbo协议
	if(Dubbo == BizProtocolType){
		icmpLayer := packet.Layer(layers.LayerTypeTCP)
		var content = icmpLayer.LayerPayload()
		if len(content) <= 17 {
			return false;
		}
		var	payload16 = fmt.Sprintf("%x", content)
		payload16 = payload16[0:4]
		if(strings.Compare(payload16, "dabb") != 0){
			return false;
		}
	}
	return true;
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