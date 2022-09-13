package proto

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

const (
	RequestPayload          = '1'
	ResponsePayload         = '2'
	ReplayedResponsePayload = '3'
)

var PayloadSeparator = "\n🐵🙈🙉\n"

func PayloadHeader(payloadType byte, ack uint32, seq uint32) (header []byte) {
	tokenStr := string(payloadType) + ":" + strconv.Itoa(int(ack)) + ":" + strconv.Itoa(int(seq))
	token := []byte(tokenStr)
	return token
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
