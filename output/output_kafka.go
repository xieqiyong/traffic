package output

import (
	"encoding/json"
	"fmt"
	"log"
	"perfma-replay/byteutils"
	"perfma-replay/message"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
)

// KafkaOutput is used for sending payloads to kafka in JSON format.
type KafkaOutput struct {
	config   *OutputKafkaConfig
	producer sarama.AsyncProducer
}



// KafkaOutputFrequency in milliseconds
const KafkaOutputFrequency = 500

// NewKafkaOutput creates instance of kafka producer client  with TLS config
func NewKafkaOutput(address string, config *OutputKafkaConfig, tlsConfig *KafkaTLSConfig) * KafkaOutput{
	c := NewKafkaConfig(tlsConfig)

	var producer sarama.AsyncProducer

	if mock, ok := config.producer.(*mocks.AsyncProducer); ok && mock != nil {
		producer = config.producer
	} else {
		c.Producer.RequiredAcks = sarama.WaitForLocal
		c.Producer.Compression = sarama.CompressionSnappy
		c.Producer.Flush.Frequency = KafkaOutputFrequency * time.Millisecond

		brokerList := strings.Split(config.Host, ",")

		var err error
		producer, err = sarama.NewAsyncProducer(brokerList, c)
		if err != nil {
			log.Fatalln("Failed to start Sarama(Kafka) producer:", err)
		}
	}

	o := &KafkaOutput{
		config:   config,
		producer: producer,
	}

	// Start infinite loop for tracking errors for kafka producer.
	go o.ErrorHandler()

	return o
}

// ErrorHandler should receive errors
func (o *KafkaOutput) ErrorHandler() {
	for err := range o.producer.Errors() {
		fmt.Printf( "Failed to write access log entry: %v", err)
	}
}

// PluginWrite writes a message to this plugin
func (o *KafkaOutput) Write(data []byte) (n int, err error) {
	var assembleData = message.DubboOutPutFile{};
	jsonMessage, _ := json.Marshal(&assembleData)
	var message sarama.StringEncoder
	message = sarama.StringEncoder(byteutils.SliceToString(jsonMessage))
	o.producer.Input() <- &sarama.ProducerMessage{
		Topic: o.config.Topic,
		Value: message,
	}
	return len(message), nil
}
