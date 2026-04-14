//go:build !no_ibex

package ibex

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/client"
	"flashcat.cloud/categraf/ibex/types"
	"k8s.io/klog/v2"
)

func heartbeatCron(ctx context.Context, ib *config.IbexConfig) {
	klog.InfoS("ibex agent start rolling request", "rpc", "Server.Report")
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
		klog.ErrorS(err, "rpc call failed", "rpc", "Server.Report")
		client.CloseCli()
		return
	}

	if resp.Message != "" {
		klog.ErrorS(nil, "error from server", "rpc", "Server.Report", "message", resp.Message)
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
		klog.InfoS("assigned tasks", "task_ids", mapKeys(assigned))
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
		klog.InfoS("ibex agent received signal", "signal", sig.String())
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
