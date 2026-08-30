//go:build !no_ibex

package ibex

import (
	"context"
	"log"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/client"
	"flashcat.cloud/categraf/ibex/types"
)

const shutdownTimeout = 10 * time.Second

var lifecycle struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func heartbeatCron(ctx context.Context, ib *config.IbexConfig) {
	log.Println("I! ibex agent start rolling request Server.Report.")
	interval := time.Duration(ib.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			heartbeat(ctx)
		}
	}
}

func heartbeat(ctx context.Context) {
	ident := config.Config.GetHostname()
	req := types.ReportRequest{
		Ident:       ident,
		ReportTasks: Locals.ReportTasks(),
	}

	var resp types.ReportResponse

	err := client.Call("Server.Report", req, &resp)

	if err != nil {
		log.Println("E! rpc call Server.Report fail:", err)
		client.CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E! error from server:", resp.Message)
		return
	}
	if ctx.Err() != nil {
		return
	}

	assigned := make(map[int64]struct{})

	if resp.AssignTasks != nil {
		count := len(resp.AssignTasks)
		for i := 0; i < count; i++ {
			at := resp.AssignTasks[i]
			assigned[at.Id] = struct{}{}
			Locals.AssignTask(at)
		}
	}

	if len(assigned) > 0 {
		log.Println("I! assigned tasks:", mapKeys(assigned))
	}

	Locals.Clean(assigned)
}

func mapKeys(m map[int64]struct{}) []int64 {
	lst := make([]int64, 0, len(m))
	for k := range m {
		lst = append(lst, k)
	}
	return lst
}

func Start() {
	lifecycle.Lock()
	if lifecycle.cancel != nil {
		lifecycle.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lifecycle.cancel = cancel
	lifecycle.done = done
	lifecycle.Unlock()

	Locals.StartAccepting()
	go func() {
		defer close(done)
		heartbeatCron(ctx, config.Config.Ibex)
	}()
}

func Stop() {
	started := time.Now()
	lifecycle.Lock()
	cancel := lifecycle.cancel
	done := lifecycle.done
	lifecycle.cancel = nil
	lifecycle.done = nil
	lifecycle.Unlock()
	if cancel != nil {
		cancel()
	}

	Locals.StopAll(shutdownTimeout)
	if done == nil {
		return
	}
	remaining := shutdownTimeout - time.Since(started)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		log.Println("W! timed out waiting for ibex heartbeat worker during shutdown")
	}
}
