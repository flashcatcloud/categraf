package agent

import (
	"fmt"
	"path"
	"strconv"

	"github.com/toolkits/pkg/file"
)

type Agent struct {
	ConfigDir string
	DebugMode bool
}

func NewAgent(configDir, debugMode string) (*Agent, error) {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return nil, fmt.Errorf("configuration file(%s) not found", configFile)
	}

	debug, err := strconv.ParseBool(debugMode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bool(%s): %v", debugMode, err)
	}

	return &Agent{
		ConfigDir: configDir,
		DebugMode: debug,
	}, nil
}

func (a *Agent) String() string {
	return fmt.Sprintf("<ConfigDir:%s DebugMode:%v>", a.ConfigDir, a.DebugMode)
}
