package main

import (
	"perfma-replay/input"
	"perfma-replay/message"
	"perfma-replay/output"
	"reflect"
	"strings"
)

// InOutPlugins struct for holding references to plugins
type InOutPlugins struct {
	Inputs  []message.PluginReader
	Outputs []message.PluginWriter
	All     []interface{}
}

// Plugins holds all the plugin objects
//var Plugins = new(InOutPlugins)

// extractLimitOptions detects if plugin get called with limiter support
// Returns address and limit
func extractLimitOptions(options string) (string, string) {
	split := strings.Split(options, "|")

	if len(split) > 1 {
		return split[0], split[1]
	}

	return split[0], ""
}

// Automatically detects type of plugin and initialize it
//
// See this article if curious about reflect stuff below: http://blog.burntsushi.net/type-parametric-functions-golang
func (plugins *InOutPlugins)registerPlugin(constructor interface{}, options ...interface{}) {
	var path, limit string
	vc := reflect.ValueOf(constructor)

	// Pre-processing options to make it work with reflect
	vo := []reflect.Value{}
	for _, oi := range options {
		vo = append(vo, reflect.ValueOf(oi))
	}

	if len(vo) > 0 {
		// Removing limit options from path
		path, limit = extractLimitOptions(vo[0].String())

		// Writing value back without limiter "|" options
		vo[0] = reflect.ValueOf(path)
	}

	// Calling our constructor with list of given options
	plugin := vc.Call(vo)[0].Interface()

	if limit != "" {
		plugin = NewLimiter(plugin, limit)
	}
	if r, ok := plugin.(message.PluginReader); ok {
		plugins.Inputs = append(plugins.Inputs, r)
	}
	if w, ok := plugin.(message.PluginWriter); ok {
		plugins.Outputs = append(plugins.Outputs, w)
	}
	plugins.All = append(plugins.All, plugin)
}

// InitPlugins specify and initialize all available plugins
func InitPlugins() *InOutPlugins{
	plugins := new(InOutPlugins)
	for _, options := range Settings.inputTCP {
		plugins.registerPlugin(input.NewTCPInput, options, false)
	}

	for _, options := range Settings.inputDubbo {
		plugins.registerPlugin(input.NewDubboMessage, options, false)
	}

	for _, options := range Settings.inputHttp {
		plugins.registerPlugin(input.NewHttpMessage, options, Settings.trackResponse, Settings.CopyBufferSize)
	}
	for _, options := range Settings.inputFile {
		plugins.registerPlugin(input.NewFileInput, options, Settings.inputFileLoop)
	}
	if Settings.outputStdout {
		plugins.registerPlugin(output.NewStdOutput)
	}

	for _, options := range Settings.outputFile {
		plugins.registerPlugin(output.NewFileOutput, options, &Settings.outputFileConfig)
	}

	for _, options := range Settings.outputResponseFile {
		plugins.registerPlugin(output.NewFileResponseOutput, options, &Settings.outputFileConfig)
	}

	for _, options := range Settings.outputTCP {
		plugins.registerPlugin(output.NewTCPOutput, options, &Settings.tcpOutputConfig)
	}

	if Settings.outputKafkaConfig.Topic != "" && Settings.outputKafkaConfig.Host != "" {
		plugins.registerPlugin(output.NewKafkaOutput, "", &Settings.outputKafkaConfig, &Settings.KafkaTLSConfig)
	}
	return plugins
}
