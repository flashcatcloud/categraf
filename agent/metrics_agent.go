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
		input := creator()

		// set configurations for input instance
		err = cfg.LoadConfigs(path.Join(config.Config.ConfigDir, inputFilePrefix+name), input)
		if err != nil {
			log.Println("E! failed to load configuration of plugin:", name, "error:", err)
			continue
		}

		if err = input.Init(); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				log.Println("E! failed to init input:", name, "error:", err)
			}
			continue
		}

		if input.GetInstances() != nil {
			instances := input.GetInstances()
			if len(instances) == 0 {
				continue
			}

			empty := true
			for i := 0; i < len(instances); i++ {
				err := instances[i].Init()
				if err != nil {
					if !errors.Is(err, types.ErrInstancesEmpty) {
						log.Println("E! failed to init input:", name, "error:", err)
					}
					continue
				}

				empty = false
			}

			if empty {
				continue
			}
		}

		a.StartInputReader(name, input)
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
