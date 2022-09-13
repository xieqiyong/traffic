package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"perfma-replay/listener"
	"runtime"
	"syscall"
	"time"
)

var closeCh chan int

func main() {
	//err := Settings.inputHttp.Set(":17922")
	//Settings.outputFileConfig.Append = false;
	//Settings.outputFileConfig.SizeLimit = 32
	//Settings.outputFileConfig.QueueLimit = 30
	//Settings.outputFile.Set("/Users/liusu/Documents/request.json")
	//Settings.bizProtocol = [] string {"http"}
	//if err != nil {
	//	return
	//}
	//Settings.outputStdout = true;
	//Settings.trackResponse = true;
	// add line number to log
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU() * 2)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if !flag.Parsed() {
		flag.Parse()
	}

	InitPlugins()
	fmt.Printf("input and output nums: %d - %d\n", len(Plugins.Inputs), len(Plugins.Outputs))

	if len(Plugins.Inputs) == 0 || len(Plugins.Outputs) == 0 {
		log.Fatal("Required at least 1 input and 1 output")
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		finalize()
		os.Exit(1)
	}()

	if Settings.exitAfter > 0 {
		log.Println("Running gor for a duration of", Settings.exitAfter)
		closeCh = make(chan int)

		time.AfterFunc(Settings.exitAfter, func() {
			log.Println("Stopping gor after", Settings.exitAfter)
			close(closeCh)
		})
	}
	// 传递协议
	listener.BizProtocolType = Settings.bizProtocol[0]
	Start(closeCh)

}

func finalize() {
	for _, p := range Plugins.All {
		if cp, ok := p.(io.Closer); ok {
			cp.Close()
		}
	}
}
