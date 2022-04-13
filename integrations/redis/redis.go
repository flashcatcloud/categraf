package redis

import (
	"log"
	"time"

	"flashcat.cloud/categraf/integrations/common"
)

var (
	DefaultInterval = time.Second * 30
	DefaultTimeout  = time.Second * 20
)

type Target struct {
	common.IntervalDuration
	common.TimeoutDuration
	Address  string
	Password string
	quit     chan struct{}
	Labels   map[string]string
}

type Redis struct {
	common.IntervalDuration
	common.TimeoutDuration
	Targets []*Target
	Labels  map[string]string
}

// overwrite func
func (r *Redis) Description() string {
	return "Read metrics from one or many redis servers"
}

// overwrite func
func (r *Redis) TidyConfig() error {
	if err := r.IntervalDuration.Tidy(); err != nil {
		return err
	}

	if err := r.TimeoutDuration.Tidy(); err != nil {
		return err
	}

	if len(r.Targets) == 0 {
		log.Println("I! [redis] Targets is empty")
	}

	for i := 0; i < len(r.Targets); i++ {
		if err := r.Targets[i].IntervalDuration.Tidy(); err != nil {
			return err
		}

		if err := r.Targets[i].TimeoutDuration.Tidy(); err != nil {
			return err
		}
	}

	return nil
}

// overwrite func
func (r *Redis) StopGoroutines() {
	for i := 0; i < len(r.Targets); i++ {
		close(r.Targets[i].quit)
	}
}

// overwrite func
func (r *Redis) StartGoroutines() {
	for i := 0; i < len(r.Targets); i++ {
		r.Targets[i].quit = make(chan struct{})
		go r.Targets[i].LoopGather(r)
	}
}

func (t *Target) getInterval(r *Redis) time.Duration {
	if t.IntervalDur != 0 {
		return r.IntervalDur
	}

	if r.IntervalDur != 0 {
		return r.IntervalDur
	}

	return DefaultInterval
}

func (t *Target) LoopGather(r *Redis) {
	for {
		select {
		case <-t.quit:
			return
		default:
			time.Sleep(t.getInterval(r))
			t.Gather(r)
		}
	}
}

func (t *Target) Gather(r *Redis) {
}
