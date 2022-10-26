//go:build windows
// +build windows

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"flashcat.cloud/categraf/agent"
	"github.com/chai2010/winsvc"
)

var (
	flagWinSvcName      = flag.String("win-service-name", "categraf", "Set windows service name")
	flagWinSvcDesc      = flag.String("win-service-desc", "Categraf", "Set windows service description")
	flagWinSvcInstall   = flag.Bool("win-service-install", false, "Install windows service")
	flagWinSvcUninstall = flag.Bool("win-service-uninstall", false, "Uninstall windows service")
	flagWinSvcStart     = flag.Bool("win-service-start", false, "Start windows service")
	flagWinSvcStop      = flag.Bool("win-service-stop", false, "Stop windows service")
)

func runAgent(ag *agent.Agent) {
	if !winsvc.IsAnInteractiveSession() {
		f, err := os.OpenFile("categraf.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln("F! failed to open log file: categraf.log")
		}

		defer f.Close()

		log.SetOutput(f)

		if err := winsvc.RunAsService(*flagWinSvcName, ag.Start, ag.Stop, false); err != nil {
			log.Fatalln("F! failed to run windows service:", err)
		}
		return
	}

	ag.Start()
	handleSignal(ag)
}

func doOSsvc() {
	// install service
	if *flagWinSvcInstall && runtime.GOOS == "windows" {
		if err := winsvc.InstallService(appPath, *flagWinSvcName, *flagWinSvcDesc); err != nil {
			log.Fatalln("F! failed to install service:", *flagWinSvcName, "error:", err)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// uninstall service
	if *flagWinSvcUninstall && runtime.GOOS == "windows" {
		if err := winsvc.RemoveService(*flagWinSvcName); err != nil {
			log.Fatalln("F! failed to uninstall service:", *flagWinSvcName, "error:", err)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// start service
	if *flagWinSvcStart && runtime.GOOS == "windows" {
		if err := winsvc.StartService(*flagWinSvcName); err != nil {
			log.Fatalln("F! failed to start service:", *flagWinSvcName, "error:", err)
		}
		fmt.Println("done")
		os.Exit(0)
	}

	// stop service
	if *flagWinSvcStop && runtime.GOOS == "windows" {
		if err := winsvc.StopService(*flagWinSvcName); err != nil {
			log.Fatalln("F! failed to stop service:", *flagWinSvcName, "error:", err)
		}
		fmt.Println("done")
		os.Exit(0)
	}
}
