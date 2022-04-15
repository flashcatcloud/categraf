package agent

import (
	"fmt"
	"log"
	"path"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/toolkits/pkg/file"

	// auto registry
	_ "flashcat.cloud/categraf/inputs/mem"
	_ "flashcat.cloud/categraf/inputs/redis"
	_ "flashcat.cloud/categraf/inputs/system"
)

type Agent struct{}

func NewAgent(configDir, debugMode string, testMode bool) (*Agent, error) {
	if err := config.InitConfig(configDir, debugMode, testMode); err != nil {
		return nil, fmt.Errorf("failed to init config: %v", err)
	}

	// init writers
	if err := writer.Init(config.Config.Writers); err != nil {
		return nil, fmt.Errorf("failed to init writers: %v", err)
	}

	return &Agent{}, nil
}

func (a *Agent) Start() {
	log.Println("I! agent starting")

	a.startInputs()
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")

	for name := range InputReaders {
		InputReaders[name].Instance.StopGoroutines()
		close(InputReaders[name].Queue)
	}
}

func (a *Agent) Reload() {
	log.Println("I! agent reloading")

	a.Stop()
	a.Start()
}

func (a *Agent) startInputs() error {
	names, err := a.getInputsByDirs()
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		creator, has := inputs.InputCreators[name]
		if !has {
			log.Println("E! input:", name, "not supported")
			continue
		}

		// construct input instance
		instance := creator()

		// set configurations for input instance
		cfg.LoadConfigs(path.Join(config.Config.ConfigDir, "input."+name), instance)

		// check configurations
		if err = instance.TidyConfig(); err != nil {
			log.Println("E! input:", name, "configurations invalid:", err)
			continue
		}

		c := &Reader{
			Instance: instance,
			Queue:    make(chan *types.Sample, 1000000),
		}

		log.Println("I! input:", name, "started")
		c.Start()

		InputReaders[name] = c
	}

	return nil
}

// input dir should has prefix input.
func (a *Agent) getInputsByDirs() ([]string, error) {
	dirs, err := file.DirsUnder(config.Config.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", config.Config.ConfigDir, err)
	}

	count := len(dirs)
	if count == 0 {
		return dirs, nil
	}

	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if strings.HasPrefix(dirs[i], "input.") {
			names = append(names, dirs[i][6:])
		}
	}

	return names, nil
}
