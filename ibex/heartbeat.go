//go:build !no_ibex

package ibex

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/client"
	"flashcat.cloud/categraf/ibex/types"
)

func heartbeatCron(ctx context.Context, ib *config.IbexConfig) {
	log.Println("I! ibex agent start rolling request Server.Report.")
	interval := time.Duration(ib.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			heartbeat()
		}
	}
}

func heartbeat() {
	ident := config.Config.GetHostname()
	req := types.ReportRequest{
		Ident:       ident,
		ReportTasks: Locals.ReportTasks(),
	}

	var resp types.ReportResponse
	err := client.GetCli().Call("Server.Report", req, &resp)

	if err != nil {
		log.Println("E! rpc call Server.Report fail:", err)
		client.CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E! error from server:", resp.Message)
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
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeatCron(ctx, config.Config.Ibex)

EXIT:
	for {
		sig := <-sc
		log.Println("I! ibex agent received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			break EXIT
		case syscall.SIGHUP:
			break EXIT
		default:
			break EXIT
		}
	}

	cancel()
}

func Stop() {
}
