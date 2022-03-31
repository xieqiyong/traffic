// Package byteutils provides helpers for working with byte slices
package byteutils

import (
	"bytes"
	"encoding/gob"
	"unsafe"
)

// Cut elements from slice for a given range
func Cut(a []byte, from, to int) []byte {
	copy(a[from:], a[to:])
	a = a[:len(a)-to+from]

	return a
}

// Insert new slice at specified position
func Insert(a []byte, i int, b []byte) []byte {
	a = append(a, make([]byte, len(b))...)
	copy(a[i+len(b):], a[i:])
	copy(a[i:i+len(b)], b)

	return a
}

// Replace function unlike bytes.Replace allows you to specify range
func Replace(a []byte, from, to int, new []byte) []byte {
	lenDiff := len(new) - (to - from)

	if lenDiff > 0 {
		// Extend if new segment bigger
		a = append(a, make([]byte, lenDiff)...)
		copy(a[to+lenDiff:], a[to:])
		copy(a[from:from+len(new)], new)

		return a
	}

	if lenDiff < 0 {
		copy(a[from:], new)
		copy(a[from+len(new):], a[to:])
		return a[:len(a)+lenDiff]
	}

	// same size
	copy(a[from:], new)
	return a
}

// SliceToString preferred for large body payload (zero allocation and faster)
func SliceToString(buf []byte) string {
	return *(*string)(unsafe.Pointer(&buf))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
