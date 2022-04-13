package inputs

import (
	"flashcat.cloud/categraf/types"
)

type Input interface {
	TidyConfig() error
	StartGoroutines(chan *types.Sample)
	StopGoroutines()
}

type Creator func() Input

var InputCreators = map[string]Creator{}

func Add(name string, creator Creator) {
	InputCreators[name] = creator
}
