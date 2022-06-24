package agent

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/file"
)

const inputFilePrefix = "input."

func (a *Agent) startMetricsAgent() error {
	names, err := a.getInputsByDirs()
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		if len(a.InputFilters) > 0 {
			// do filter
			if _, has := a.InputFilters[name]; !has {
				continue
			}
		}

		creator, has := inputs.InputCreators[name]
		if !has {
			log.Println("E! input:", name, "not supported")
			continue
		}

		// construct input instance
		instance := creator()
		// set configurations for input instance
		cfg.LoadConfigs(path.Join(config.Config.ConfigDir, inputFilePrefix+name), instance)

		if err = instance.Init(); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				log.Println("E! failed to init input:", name, "error:", err)
			}
			continue
		}

		reader := NewInputReader(instance)
		reader.Start()
		a.InputReaders[name] = reader

		log.Println("I! input:", name, "started")
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
		if strings.HasPrefix(dirs[i], inputFilePrefix) {
			names = append(names, dirs[i][len(inputFilePrefix):])
		}
	}

	return names, nil
}

func (a *Agent) stopMetricsAgent() {
	for name := range a.InputReaders {
		a.InputReaders[name].Stop()
	}
}
