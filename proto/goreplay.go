package proto

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"perfma-replay/core"
)

const (
	RequestPayload          = '1'
	ResponsePayload         = '2'
	ReplayedResponsePayload = '3'
)

var PayloadSeparator = "\n🐵🙈🙉\n"

func PayloadHeader(payloadType byte, uuid []byte) (header []byte) {
	return []byte(fmt.Sprintf("%c %s\n", payloadType, uuid))
}

func PayloadBody(payload []byte) []byte {
	headerSize := bytes.IndexByte(payload, '\n')
	return payload[headerSize+1:]
}

func PayloadMeta(payload []byte) [][]byte {
	headerSize := bytes.IndexByte(payload, '\n')
	if headerSize < 0 {
		headerSize = 0
	}
	return bytes.Split(payload[:headerSize], []byte{' '})
}

func IsRequestPayload(payload []byte) bool {
	return payload[0] == RequestPayload
}

func RandByte(len int) []byte {
	b := make([]byte, len/2)
	rand.Read(b)

	h := make([]byte, len)
	hex.Encode(h, b)

	return h
}

func UUID(c *core.NewPacket, isRequest bool) []byte {
	var streamID uint64

	// check if response or request have generated the ID before.
	if isRequest == true {
		streamID = uint64(c.SrcPort)<<48 | uint64(c.DstPort)<<32 |
			uint64(ip2int(c.SrcIP))
	} else {
		streamID = uint64(c.DstPort)<<48 | uint64(c.SrcPort)<<32 |
			uint64(ip2int(c.DstIP))
	}

	id := make([]byte, 12)
	binary.BigEndian.PutUint64(id, streamID)

	if isRequest == true {
		binary.BigEndian.PutUint32(id[8:], c.Ack)
	} else {
		binary.BigEndian.PutUint32(id[8:], c.Seq)
	}

	uuidHex := make([]byte, 24)
	hex.Encode(uuidHex[:], id[:])

	return uuidHex
}

func ip2int(ip net.IP) uint32 {
	if len(ip) == 0 {
		return 0
	}

	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}
