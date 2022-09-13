package main

import (
	"io"
	"perfma-replay/message"
	"time"
)

// Start initialize loop for sending data from inputs to outputs
func Start(stop chan int) {

	for _, in := range Plugins.Inputs {
		go CopyMulty(in, Plugins.Outputs...)
	}

	for {
		select {
		case <-stop:
			finalize()
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// CopyMulty copies from 1 reader to multiple writers
func CopyMulty(src message.PluginReader, writers ...message.PluginWriter) (err error) {
	if Settings.CopyBufferSize < 1 {
		Settings.CopyBufferSize = 5 << 20
	}
	for {
		msg, er := src.PluginReader()
		if er != nil {
			err = er
			break
		}
		if msg != nil && len(msg.Data) > 0 {
			if len(msg.Data) > int(Settings.CopyBufferSize) {
				msg.Data = msg.Data[:Settings.CopyBufferSize]
			}
			for _, dst := range writers {
				if _, err := dst.PluginWriter(msg); err != nil && err != io.ErrClosedPipe {
					return err
				}
			}
		}
	}

	return err
}
