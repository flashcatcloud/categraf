package agent

import (
	"errors"
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

func (a *Agent) startMetricsAgent() error {
	var ip inputs.InputProvider
	if config.Config.Global.InputProvider == "TemplateInputProvider" {
		log.Println("I! use TemplateInputProvider config plugins, please make sure template and context are set appropriately")
		ip = &inputs.TemplateInputProvider{ConfigDir: config.Config.ConfigDir, ContextMap: config.Config.Context}
	} else {
		ip = &inputs.DirConfigInputProvider{ConfigDir: config.Config.ConfigDir}
	}

	names, err := ip.ListInputNames()
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

		inp := ip.GetInput(creator, name)
		if err = inp.Init(); err != nil {
			if !errors.Is(err, types.ErrInstancesEmpty) {
				log.Println("E! failed to init input:", name, "error:", err)
			}
			continue
		}

		reader := NewInputReader(inp)
		reader.Start(name)
		a.InputReaders[name] = reader

		log.Println("I! input:", name, "started")
	}

	return nil
}

func (a *Agent) stopMetricsAgent() {
	for name := range a.InputReaders {
		a.InputReaders[name].Stop()
	}
}
