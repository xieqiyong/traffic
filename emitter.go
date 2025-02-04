package main

import (
	"fmt"
	"io"
	"perfma-replay/byteutils"
	"perfma-replay/message"
	"perfma-replay/modifier"
	"perfma-replay/proto"
	"sync"
	"time"
)

func NewEmitter() *Emitter {
	return &Emitter{}
}

type Emitter struct {
	sync.WaitGroup
	plugins *InOutPlugins
}
// Start initialize loop for sending data from inputs to outputs
func (e *Emitter) Start(plugins *InOutPlugins) {

	if Settings.CopyBufferSize < 1 {
		Settings.CopyBufferSize = 5 << 20
	}
	e.plugins = plugins

	for _, in := range plugins.Inputs {
		e.Add(1)
		go func(in message.PluginReader) {
			defer e.Done()
			if err := CopyMulty(in, plugins.Outputs...); err != nil {
				Debug(2, fmt.Sprintf("[EMITTER] error during copy: %q", err))
			}
		}(in)
	}
}

func (e *Emitter) Close() {
	for _, p := range e.plugins.All {
		if cp, ok := p.(io.Closer); ok {
			cp.Close()
		}
	}
	if len(e.plugins.All) > 0 {
		// wait for everything to stop
		e.Wait()
	}
	e.plugins.All = nil // avoid Close to make changes again
}

// CopyMulty copies from 1 reader to multiple writers
func CopyMulty(src message.PluginReader, writers ...message.PluginWriter) (err error) {
	filteredCount := 0
	modifierRule := modifier.NewHTTPModifier(&Settings.modifierConfig)
	filteredRequestsLastCleanTime := time.Now().UnixNano()
	filteredRequests := make(map[string]int64)
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
			meta := proto.PayloadMeta(msg.Meta)

			if len(meta) < 3 {
				Debug(2, fmt.Sprintf("[EMITTER] Found malformed record %q from %q", msg.Meta, src))
				continue
			}
			requestID := byteutils.SliceToString(meta[1])
			if Settings.Verbose >= 3 {
				Debug(3, "[EMITTER] input: ", byteutils.SliceToString(msg.Meta[:len(msg.Meta)-1]), " from: ", src)
			}
			if(modifierRule != nil){
				if(proto.IsRequestPayload(msg.Meta)){
					msg.Data = modifierRule.Rewrite(msg.Data)
					if(len(msg.Data) == 0){
						filteredRequests[requestID] = time.Now().UnixNano()
						filteredCount++
						continue
					}
				}else{
					if _, ok := filteredRequests[requestID]; ok {
						delete(filteredRequests, requestID)
						filteredCount--
						continue
					}
				}
			}

			for _, dst := range writers {
				if _, err := dst.PluginWriter(msg); err != nil && err != io.ErrClosedPipe {
					return err
				}
			}
		}
		// Run GC on each 1000 request
		if filteredCount > 0 && filteredCount%1000 == 0 {
			// Clean up filtered requests for which we didn't get a response to filter
			now := time.Now().UnixNano()
			if now-filteredRequestsLastCleanTime > int64(60*time.Second) {
				for k, v := range filteredRequests {
					if now-v > int64(60*time.Second) {
						delete(filteredRequests, k)
						filteredCount--
					}
				}
				filteredRequestsLastCleanTime = time.Now().UnixNano()
			}
		}
	}

	return err
}
