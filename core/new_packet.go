package core

import (
	"encoding/binary"
	"log"
	"net"
	"time"

	"github.com/google/gopacket"
)

type NewPacket struct {
	messageID          uint64
	SrcIP, DstIP       net.IP
	Version            uint8
	SrcPort, DstPort   uint16
	Ack, Seq           uint32
	ACK, SYN, FIN, RST bool
	Lost               uint32
	Retry              int
	CaptureLength      int
	Timestamp          time.Time
	Payload            []byte
	buf                []byte

	created time.Time
	gc      bool
}

func (newPacket *NewPacket) Parse(data []byte, lType, lTypeLen int, cp *gopacket.CaptureInfo, allowEmpty bool) bool {
	newPacket.Retry = 0
	newPacket.messageID = 0
	newPacket.buf = newPacket.buf[:]

	// TODO: check resolution
	newPacket.Timestamp = cp.Timestamp

	if len(data) < lTypeLen {
		log.Println("Link")
		return false
	}
	if len(data) <= lTypeLen {
		log.Println("IPv4 or IPv6")
		return false
	}

	ldata := data[lTypeLen:]
	var proto byte
	var netLayer, transLayer []byte
	if ldata[0]>>4 == 4 {
		// IPv4 header
		if len(ldata) < 20 {
			log.Println("IPv4")
			return false
		}
		proto = ldata[9]
		ihl := int(ldata[0]&0x0F) * 4
		if ihl < 20 {
			log.Println("IPv4's IHL")
			return false
		}
		if len(ldata) < ihl {
			log.Println("IPv4 opts")
			return false
		}
		netLayer = ldata[:ihl]
	} else if ldata[0]>>4 == 6 {
		if len(ldata) < 40 {
			log.Println("IPv6")
			return false
		}
		proto = ldata[6]
		totalLen := 40
		for ipv6ExtensionHdr(proto) {
			hdr := len(ldata) - totalLen
			if hdr < 8 {
				log.Println("IPv6 opts")
				return false
			}
			extLen := 8
			if proto != 44 {
				extLen = int(ldata[totalLen+1]+1) * 8
			}
			if hdr < extLen {
				log.Println("IPv6 opts")
				return false
			}
			proto = ldata[totalLen]
			totalLen += extLen
		}
		netLayer = ldata[:totalLen]
	} else {
		log.Println("IPv4 or IPv6")
		return false
	}
	if proto != 6 {
		log.Println("TCP")
		return false
	}
	if len(data) <= len(netLayer) {
		log.Println("TCP")
		return false
	}
	ndata := ldata[len(netLayer):]
	// TCP header
	if len(ndata) < 20 {
		log.Println("TCP")
		return false
	}
	dOf := int(ndata[12]>>4) * 4
	if dOf < 20 {
		log.Println("TCP's ndata offset")
		return false
	}
	if len(ndata) < dOf {
		log.Println("TCP opts")
		return false
	}

	// There are case when core have padding but dOf shows its not
	empty := true
	for i := 0; i < len(ndata[dOf:]); i++ {
		if ndata[dOf:][i] != 0 {
			empty = false
			break
		}
	}

	if !allowEmpty && empty {
		return false

	}

	if (netLayer[0] >> 4) == 4 {
		// IPv4 header
		newPacket.Version = 4
		newPacket.SrcIP = netLayer[12:16]
		newPacket.DstIP = netLayer[16:20]
	} else {
		// IPv6 header
		newPacket.Version = 6
		newPacket.SrcIP = netLayer[8:24]
		newPacket.DstIP = netLayer[24:40]
	}

	transLayer = ndata[:dOf]

	newPacket.CaptureLength = cp.CaptureLength
	newPacket.SrcPort = binary.BigEndian.Uint16(transLayer[0:2])
	newPacket.DstPort = binary.BigEndian.Uint16(transLayer[2:4])
	newPacket.Seq = binary.BigEndian.Uint32(transLayer[4:8])
	newPacket.Ack = binary.BigEndian.Uint32(transLayer[8:12])
	newPacket.FIN = transLayer[13]&0x01 != 0
	newPacket.SYN = transLayer[13]&0x02 != 0
	newPacket.RST = transLayer[13]&0x04 != 0
	newPacket.ACK = transLayer[13]&0x10 != 0
	newPacket.Lost = uint32(cp.Length - cp.CaptureLength)
	newPacket.Payload = ndata[dOf:]
	return true
}

func ipv6ExtensionHdr(b byte) bool {
	// TODO: support all extension headers
	return b == 0 || b == 43 || b == 44
}
