package inputs

import (
	"flashcat.cloud/categraf/config"
	"github.com/toolkits/pkg/container/list"
)

type Input interface {
	Init() error
	Drop()
	Prefix() string
	GetInterval() config.Duration
	Gather(slist *list.SafeList)
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
	Init() error
	Gather(slist *list.SafeList)
}
