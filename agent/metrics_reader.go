package agent

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/house"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/runtimex"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
)

type InputReader struct {
	inputName  string
	input      inputs.Input
	quitChan   chan struct{}
	runCounter uint64
	waitGroup  sync.WaitGroup
}

func (a *Agent) RegisterInput(name string, configs []cfg.ConfigWithFormat) {
	_, inputKey := inputs.ParseInputName(name)
	if !a.FilterPass(inputKey) {
		return
	}

	creator, has := inputs.InputCreators[inputKey]
	if !has {
		log.Println("E! input:", name, "not supported")
		return
	}

	// construct input instance
	input := creator()

	err := cfg.LoadConfigs(configs, input)
	if err != nil {
		log.Println("E! failed to load configuration of plugin:", name, "error:", err)
		return
	}

	if err = input.InitInternalConfig(); err != nil {
		log.Println("E! failed to init input:", name, "error:", err)
		return
	}

	if err = inputs.MayInit(input); err != nil {
		if !errors.Is(err, types.ErrInstancesEmpty) {
			log.Println("E! failed to init input:", name, "error:", err)
		}
		return
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
			return
		}
	}

	reader := newInputReader(name, input)
	go reader.startInput()
	a.InputReaders[name] = reader
	log.Println("I! input:", name, "started")
}

func (a *Agent) DeregisterInput(name string) {
	if reader, has := a.InputReaders[name]; has {
		reader.Stop()
		delete(a.InputReaders, name)
		log.Println("I! input:", name, "stopped")
	} else {
		log.Printf("W! dereigster input name [%s] not found", name)
	}
}

func (a *Agent) ReregisterInput(name string, configs []cfg.ConfigWithFormat) {
	a.DeregisterInput(name)
	a.RegisterInput(name, configs)
}

func newInputReader(inputName string, in inputs.Input) *InputReader {
	return &InputReader{
		inputName: inputName,
		input:     in,
		quitChan:  make(chan struct{}, 1),
	}
}

func (r *InputReader) Stop() {
	r.quitChan <- struct{}{}
	inputs.MayDrop(r.input)
}

func (r *InputReader) startInput() {
	interval := config.GetInterval()
	if r.input.GetInterval() > 0 {
		interval = time.Duration(r.input.GetInterval())
	}

	for {
		select {
		case <-r.quitChan:
			close(r.quitChan)
			return
		default:
			var start time.Time
			if config.Config.DebugMode {
				start = time.Now()
				log.Println("D!", r.inputName, ": before gather once")
			}

			r.gatherOnce()

			if config.Config.DebugMode {
				log.Println("D!", r.inputName, ": after gather once,", "duration:", time.Since(start))
			}

			time.Sleep(interval)
		}
	}
}

func (r *InputReader) gatherOnce() {
	defer func() {
		if rc := recover(); rc != nil {
			log.Println("E!", r.inputName, ": gather metrics panic:", r, string(runtimex.Stack(3)))
		}
	}()

	// plugin level, for system plugins
	slist := types.NewSampleList()
	inputs.MayGather(r.input, slist)
	r.forward(r.input.Process(slist))

	instances := inputs.MayGetInstances(r.input)
	if len(instances) == 0 {
		return
	}

	atomic.AddUint64(&r.runCounter, 1)

	for i := 0; i < len(instances); i++ {
		r.waitGroup.Add(1)
		go func(ins inputs.Instance) {
			defer r.waitGroup.Done()

			it := ins.GetIntervalTimes()
			if it > 0 {
				counter := atomic.LoadUint64(&r.runCounter)
				if counter%uint64(it) != 0 {
					return
				}
			}

			insList := types.NewSampleList()
			inputs.MayGather(ins, insList)
			r.forward(ins.Process(insList))
		}(instances[i])
	}

	r.waitGroup.Wait()
}

func (r *InputReader) forward(slist *types.SampleList) {
	if slist == nil {
		return
	}
	arr := slist.PopBackAll()
	for i := 0; i < len(arr); i++ {
		writer.PushQueue(arr[i])
		house.MetricsHouse.Push(arr[i])
	}
}
