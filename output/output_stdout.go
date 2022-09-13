package output

import (
	"perfma-replay/message"
)

// StdOutput used for debugging, prints all incoming requests
type StdOutput struct {
}

// NewStdOutput constructor for StdOutput
func NewStdOutput() (i *StdOutput) {
	i = new(StdOutput)
	return
}

func (i *StdOutput) PluginWriter(msg *message.OutPutMessage) (int, error) {
	return len(msg.Data), nil
}

func (i *StdOutput) String() string {
	return "Stdout Output"
}