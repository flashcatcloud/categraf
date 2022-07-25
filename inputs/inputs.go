package inputs

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

type Input interface {
	GetLabels() map[string]string
	GetInterval() config.Duration
	InitInternalConfig() error
	Process(*types.SampleList) *types.SampleList

	Init() error
	Drop()
	Gather(*types.SampleList)
	GetInstances() []Instance
}

type Creator func() Input

var InputCreators = map[string]Creator{}

func Add(name string, creator Creator) {
	InputCreators[name] = creator
}

type Instance interface {
	GetLabels() map[string]string
	GetIntervalTimes() int64
	InitInternalConfig() error
	Process(*types.SampleList) *types.SampleList

	Init() error
	Gather(*types.SampleList)
}
