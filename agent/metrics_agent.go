package agent

import (
	"log"

	"flashcat.cloud/categraf/inputs"
)

func (a *Agent) startMetricsAgent() error {
	a.InputProvider.LoadConfig()
	a.InputProvider.StartReloader()

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
		if !a.FilterPass(inputKey) {
			continue
		}

		configs, err := a.InputProvider.GetInputConfig(name)
		if err != nil {
			log.Println("E! failed to get configuration of plugin:", name, "error:", err)
			continue
		}

		a.RegisterInput(name, configs)
	}
	return nil
}

func (a *Agent) stopMetricsAgent() {
	a.InputProvider.StopReloader()
	for name := range a.InputReaders {
		a.InputReaders[name].Stop()
	}
}
