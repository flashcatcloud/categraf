package smart

import (
	_ "embed"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"os"
)

const (
	inputName = "smart"
)

type (
	Smart struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}
)

func init() {
	// Set LC_NUMERIC to uniform numeric output from cli tools
	_ = os.Setenv("LC_NUMERIC", "en_US.UTF-8")

	inputs.Add(inputName, func() inputs.Input {
		return &Smart{}
	})
}

func (s *Smart) Clone() inputs.Input {
	return &Smart{}
}

func (s *Smart) Name() string {
	return inputName
}

func (s *Smart) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}
