package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/pkg/osx"
	"github.com/toolkits/pkg/runner"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	version   = "0.0.1"
	configDir = flag.String("configs", osx.GetEnv("CATEGRAF_CONFIGS", "conf"), "Specify configuration directory")
	debugMode = flag.String("debug", osx.GetEnv("CATEGRAF_DEBUG", "false"), "Is debug mode?")
)

func main() {
	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	printEnv()

	ag, err := agent.NewAgent(*configDir, *debugMode)
	if err != nil {
		log.Println("F! failed to new agent:", err)
		os.Exit(1)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ag.Start()

EXIT:
	for {
		sig := <-sc
		log.Println("I! received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			break EXIT
		case syscall.SIGHUP:
			ag.Reload()
		default:
			break EXIT
		}
	}

	ag.Stop()
	log.Println("I! exited")
}

func printEnv() {
	runner.Init()
	log.Println("I! runner.binarydir:", runner.Cwd)
	log.Println("I! runner.hostname:", runner.Hostname)
	log.Println("I! runner.fd_limits:", runner.FdLimits())
	log.Println("I! runner.vm_limits:", runner.VMLimits())
}
