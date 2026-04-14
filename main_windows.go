//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/chai2010/winsvc"
	"k8s.io/klog/v2"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/logging"
	"flashcat.cloud/categraf/pkg/pprof"
)

var (
	pprofStart          uint32
	flagWinSvcName      = flag.String("win-service-name", "categraf", "Set windows service name")
	flagWinSvcDesc      = flag.String("win-service-desc", "Categraf", "Set windows service description")
	flagWinSvcInstall   = flag.Bool("win-service-install", false, "Install windows service")
	flagWinSvcUninstall = flag.Bool("win-service-uninstall", false, "Uninstall windows service")
	flagWinSvcStart     = flag.Bool("win-service-start", false, "Start windows service")
	flagWinSvcStop      = flag.Bool("win-service-stop", false, "Stop windows service")
)

func runAgent(ag *agent.Agent) {
	if !winsvc.IsAnInteractiveSession() {
		if err := winsvc.RunAsService(*flagWinSvcName, ag.Start, ag.Stop, false); err != nil {
			klog.ErrorS(err, "failed to run windows service")
			logging.Sync()
			os.Exit(1)
		}
		return
	}

	ag.Start()
	go profile()
	handleSignal(ag)
}

func doOSsvc() {
	// install service
	if *flagWinSvcInstall {
		if err := winsvc.InstallService(appPath, *flagWinSvcName, *flagWinSvcDesc); err != nil {
			klog.ErrorS(err, "failed to install service", "service", *flagWinSvcName)
			logging.Sync()
			os.Exit(1)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// uninstall service
	if *flagWinSvcUninstall {
		if err := winsvc.RemoveService(*flagWinSvcName); err != nil {
			klog.ErrorS(err, "failed to uninstall service", "service", *flagWinSvcName)
			logging.Sync()
			os.Exit(1)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// start service
	if *flagWinSvcStart {
		if err := winsvc.StartService(*flagWinSvcName); err != nil {
			klog.ErrorS(err, "failed to start service", "service", *flagWinSvcName)
			logging.Sync()
			os.Exit(1)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// stop service
	if *flagWinSvcStop && runtime.GOOS == "windows" {
		if err := winsvc.StopService(*flagWinSvcName); err != nil {
			klog.ErrorS(err, "failed to stop service", "service", *flagWinSvcName)
			logging.Sync()
			os.Exit(1)
		}
		fmt.Println("done")
		os.Exit(0)
	}
}

func profile() {
	// TODO: replace with windows event
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			file := filepath.Join(config.Config.ConfigDir, ".pprof")
			if _, err := os.Stat(file); err == nil {
				if !atomic.CompareAndSwapUint32(&pprofStart, 0, 1) {
					return
				}
				go pprof.Go()
			}
		}
	}
}
