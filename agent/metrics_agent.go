package agent

import (
	"errors"
	"log"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/types"
)

func (a *Agent) startMetricsAgent() error {
	names, err := a.InputProvider.GetInputs()
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		_, inputKey := inputs.ParseInputName(name)
		if len(a.InputFilters) > 0 {
			// do filter
			if _, has := a.InputFilters[inputKey]; !has {
				continue
			}
		}

		creator, has := inputs.InputCreators[inputKey]
		if !has {
			log.Println("E! input:", name, "not supported")
			continue
		}

		// construct input instance
		input := creator()

		// set configurations for input instance
		configs, err := a.InputProvider.GetInputConfig(name)
		if err != nil {
			log.Println("E! failed to get configuration of plugin:", name, "error:", err)
			continue
		}
		err = cfg.LoadConfigs(configs, input)
		if err != nil {
			log.Println("E! failed to load configuration of plugin:", name, "error:", err)
			continue
		}

		if err = input.InitInternalConfig(); err != nil {
			log.Println("E! failed to init input:", name, "error:", err)
			continue
		}

		if err = inputs.MayInit(input); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				log.Println("E! failed to init input:", name, "error:", err)
			}
			continue
		}

		instances := inputs.MayGetInstances(input)
		if instances != nil {
			empty := true
			for i := 0; i < len(instances); i++ {
				if err := instances[i].InitInternalConfig(); err != nil {
					log.Println("E! failed to init input:", name, "error:", err)
					continue
				}

				if err := inputs.MayInit(instances[i]); err != nil {
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

func (a *Agent) stopMetricsAgent() {
	for name := range a.InputReaders {
		a.InputReaders[name].Stop()
	}
}
