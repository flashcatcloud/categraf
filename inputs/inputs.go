package inputs

import (
	"flashcat.cloud/categraf/types"
)

type PluginDescriber interface {
	// TidyConfig Validate and tidy configurations
	TidyConfig() error

	// Description returns a one-sentence description
	Description() string
}

type Input interface {
	PluginDescriber

	StartGoroutines(chan *types.Sample)
	StopGoroutines()

	GetPointer() interface{}
}

type Creator func() Input

var InputCreators = map[string]Creator{}

func Add(name string, creator Creator) {
	InputCreators[name] = creator
}
