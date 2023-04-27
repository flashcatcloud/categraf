package agent

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
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
	timer := time.NewTimer(0 * time.Second)
	defer timer.Stop()
	var start time.Time

	for {
		select {
		case <-r.quitChan:
			close(r.quitChan)
			return
		case <-timer.C:
			start = time.Now()
			if config.Config.DebugMode {
				log.Println("D!", r.inputName, ": before gather once")
			}

			r.gatherOnce()

			if config.Config.DebugMode {
				log.Println("D!", r.inputName, ": after gather once,", "duration:", time.Since(start))
			}

			next := interval - time.Since(start)
			if next < 0 {
				next = 0
			}
			timer.Reset(next)
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
		if !instances[i].Initialized() {
			continue
		}
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
	writer.WriteSamples(arr)
}
