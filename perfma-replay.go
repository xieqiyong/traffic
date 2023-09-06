package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"perfma-replay/listener"
	"runtime"
	"syscall"
	"time"
)

func main() {
	//err := Settings.inputHttp.Set(":9527")
	//Settings.outputFileConfig.Append = false;
	//Settings.outputFileConfig.SizeLimit.Set("5mb")
	//Settings.outputFileConfig.QueueLimit = 0
	////Settings.outputFileConfig.Append = true
	//Settings.outputFileConfig.FlushInterval = 5 * time.Second
	//Settings.outputFile.Set("/Users/liusu/Documents/request.json")
	//Settings.outputResponseFile.Set("/Users/liusu/Documents/response.json")
	//Settings.bizProtocol = [] string {"http"}
	//if err != nil {
	//	return
	//}
	//Settings.outputStdout = true;
	//Settings.trackResponse = true;
	//Settings.modifierConfig.URLRegexp.Set("/api/*")
	// add line number to log
	//Settings.trackResponse = false
	Settings.PcapOptions.Snaplen = true
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU() * 2)
	}
	var plugins *InOutPlugins
	flag.Parse()
	checkSettings()
	plugins = InitPlugins()
	fmt.Printf("input and output nums: %d - %d\n", len(plugins.Inputs), len(plugins.Outputs))
	if len(plugins.Inputs) == 0 || len(plugins.Outputs) == 0 {
		log.Fatal("Required at least 1 input and 1 output")
	}
	// 传递协议
	listener.BizProtocolType = Settings.bizProtocol[0]

	closeCh := make(chan int)
	emitter := NewEmitter()
	go emitter.Start(plugins)
	if Settings.exitAfter > 0 {
		log.Printf("Running gor for a duration of %s\n", Settings.exitAfter)

		time.AfterFunc(Settings.exitAfter, func() {
			log.Printf("gor run timeout %s\n", Settings.exitAfter)
			close(closeCh)
		})
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	exit := 0
	select {
	case <-c:
		exit = 1
	case <-closeCh:
		exit = 0
	}
	emitter.Close()
	os.Exit(exit)

}
