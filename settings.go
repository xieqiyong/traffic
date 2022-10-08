package main

import (
	"flag"
	"fmt"
	"os"
	"perfma-replay/listener"
	"perfma-replay/modifier"
	"perfma-replay/output"
	"perfma-replay/size"
	"sync"
	"time"
)

// MultiOption allows to specify multiple flags with same name and collects all values into array
type MultiOption []string

func (h *MultiOption) String() string {
	return fmt.Sprint(*h)
}

// Set gets called multiple times for each flag with same name
func (h *MultiOption) Set(value string) error {
	*h = append(*h, value)
	return nil
}

// AppSettings is the struct of main configuration
type AppSettings struct {
	Verbose   int           `json:"verbose"`
	exitAfter time.Duration

	splitOutput  bool
	outputStdout bool
	outputNull   bool

	inputFile        MultiOption
	inputFileLoop    bool
	outputFile       MultiOption
	outputResponseFile MultiOption
	outputFileConfig output.FileOutputConfig

	inputTCP        MultiOption
	outputTCP       MultiOption
	tcpOutputConfig output.TCPOutputConfig

	// base tcp protocol type   example：http、dubbo、redis、mysql
	bizProtocol   MultiOption

	outputKafkaConfig output.OutputKafkaConfig

	KafkaTLSConfig    output.KafkaTLSConfig

	inputDubbo       MultiOption

	inputHttp        MultiOption

	trackResponse    bool

	CopyBufferSize 	size.Size

	modifierConfig  modifier.HTTPModifierConfig
	PcapOptions  listener.PcapOptions
}

// Settings holds Goreplay configuration
var Settings AppSettings

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

func init() {
	flag.Usage = usage
	flag.DurationVar(&Settings.exitAfter, "exit-after", 0, "exit after specified duration")

	flag.BoolVar(&Settings.splitOutput, "split-output", false, "By default each output gets same traffic. If set to `true` it splits traffic equally among all outputs")

	flag.Var(&Settings.inputTCP, "input-tcp", "Capture traffic from given port (use RAW sockets and require *sudo* access):\n\t# Capture traffic from 8080 port\n\ttcp_replay --input-raw :8080 --output-stdout")

	flag.Var(&Settings.inputDubbo, "input-dubbo", "Capture traffic from given port (use RAW sockets and require *sudo* access):\n\t# Capture traffic from 8080 port\n\ttcp_replay --input-raw :8080 --output-stdout")

	flag.Var(&Settings.inputHttp, "input-http", "Capture traffic from given port (use RAW sockets and require *sudo* access):\n\t# Capture traffic from 8080 port\n\ttcp_replay --input-raw :8080 --output-stdout")


	flag.Var(&Settings.inputFile, "input-file", "Read requests from file: \n\ttcp_replay --input-file ./requests.gor --output-stdout")
	flag.BoolVar(&Settings.inputFileLoop, "input-file-loop", false, "Loop input files, useful for performance testing")

	flag.Var(&Settings.outputFile, "output-file", "Write incoming requests to file: \n\ttcp_replay --input-tcp :80 --output-file ./requests.gor")
	flag.Var(&Settings.outputResponseFile, "output-response-file", "Write incoming requests to file: \n\ttcp_replay --input-tcp :80 --output-file ./response.gor")
	flag.DurationVar(&Settings.outputFileConfig.FlushInterval, "output-file-flush-interval", time.Second, "Interval for forcing buffer flush to the file, default: 1s")
	flag.BoolVar(&Settings.outputFileConfig.Append, "output-file-append", false, "The flushed chunk is appended to existence file or not")

	flag.BoolVar(&Settings.outputStdout, "output-stdout", false, "Used for testing inputs. Just prints to console data coming from inputs")

	flag.Var(&Settings.outputTCP, "output-tcp", "Used for out put to tcp address like:\n\t tcp_replay --input-file pcap.out --output-tcp 127.0.0.1:4000")
	flag.BoolVar(&Settings.tcpOutputConfig.Secure, "output-tcp-secure", false, "Use TLS secure connection. --input-file on another end should have TLS turned on as well.")
	flag.BoolVar(&Settings.tcpOutputConfig.Stats, "output-tcp-stats", false, "Report TCP output queue stats to console every 5 seconds.")
	flag.IntVar(&Settings.tcpOutputConfig.Repeat, "output-tcp-repeat", 1, "Reapt times for each request for perf testing to .")

	// Set default
	//Settings.outputFileConfig.SizeLimit.Set("32mb")
	flag.Var(&Settings.outputFileConfig.SizeLimit, "output-file-size-limit", "Size of each chunk. Default: 32mb")
	flag.IntVar(&Settings.outputFileConfig.QueueLimit, "output-file-queue-limit", 25600, "The length of the chunk queue. Default: 25600")

	// 业务标识
	flag.Var(&Settings.bizProtocol, "biz-protocol", "filter base tcp protocol and you biz protocol")

	// 输出到kafka
	flag.StringVar(&Settings.outputKafkaConfig.Host, "output-kafka-host", "", "Read request and response stats from Kafka:\n\tgor --input-raw :8080 --output-kafka-host '192.168.0.1:9092,192.168.0.2:9092'")
	flag.StringVar(&Settings.outputKafkaConfig.Topic, "output-kafka-topic", "", "Read request and response stats from Kafka:\n\tgor --input-raw :8080 --output-kafka-topic 'kafka-log'")
	flag.BoolVar(&Settings.outputKafkaConfig.UseJSON, "output-kafka-json-format", false, "If turned on, it will serialize messages from GoReplay text format to JSON.")

	// 是否录制响应
	flag.BoolVar(&Settings.trackResponse, "input-raw-track-response", false,"track response")

	flag.Var(&Settings.CopyBufferSize, "copy-buffer-size", "Set the buffer size for an individual request (default 5MB)")

	flag.Var(&Settings.modifierConfig.URLRegexp, "http-allow-url", "A regexp to match requests against. Filter get matched against full url with domain. Anything else will be dropped:\n\t gor --input-raw :8080 --output-http staging.com --http-allow-url ^www.")
	flag.Var(&Settings.PcapOptions.BufferSize, "input-raw-buffer-size", "Controls size of the OS buffer which holds packets until they dispatched. Default value depends by system: in Linux around 2MB. If you see big package drop, increase this value.")
	flag.BoolVar(&Settings.PcapOptions.Promiscuous, "input-raw-promisc", false, "enable promiscuous mode")
	flag.BoolVar(&Settings.PcapOptions.Monitor, "input-raw-monitor", false, "enable RF monitor mode")
	flag.BoolVar(&Settings.PcapOptions.Snaplen, "input-raw-override-snaplen", false, "Override the capture snaplen to be 64k. Required for some Virtualized environments")

	Settings.CopyBufferSize = 5242880
	Settings.outputFileConfig.SizeLimit = 33554432
}
var previousDebugTime = time.Now()
var debugMutex sync.Mutex

func Debug(level int, args ...interface{}) {
	if Settings.Verbose >= level {
		debugMutex.Lock()
		defer debugMutex.Unlock()
		now := time.Now()
		diff := now.Sub(previousDebugTime)
		previousDebugTime = now
		fmt.Fprintf(os.Stderr, "[DEBUG][elapsed %s]: ", diff)
		fmt.Fprintln(os.Stderr, args...)
	}
}

func checkSettings() {
	if Settings.outputFileConfig.SizeLimit < 1 {
		Settings.outputFileConfig.SizeLimit.Set("32mb")
	}
	if Settings.CopyBufferSize < 1 {
		Settings.CopyBufferSize.Set("5mb")
	}
}
