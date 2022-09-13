package message


type OutPutMessage struct {
	Meta []byte
	Data []byte
}

type PluginReader interface {
	PluginReader() (outputMessage * OutPutMessage, err error)
}

type PluginWriter interface {
	PluginWriter(outputMessage * OutPutMessage) (n int, err error)
}