package main

import (
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/api"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/heartbeat"
	"flashcat.cloud/categraf/pkg/osx"
	"flashcat.cloud/categraf/writer"
	"github.com/chai2010/winsvc"
	"github.com/toolkits/pkg/runner"
)

var (
	appPath      string
	configDir    = flag.String("configs", osx.GetEnv("CATEGRAF_CONFIGS", "conf"), "Specify configuration directory.(env:CATEGRAF_CONFIGS)")
	debugMode    = flag.Bool("debug", false, "Is debug mode?")
	testMode     = flag.Bool("test", false, "Is test mode? print metrics to stdout")
	interval     = flag.Int64("interval", 0, "Global interval(unit:Second)")
	showVersion  = flag.Bool("version", false, "Show version.")
	inputFilters = flag.String("inputs", "", "e.g. cpu:mem:system")
)

func init() {
	// change to current dir
	var err error
	if appPath, err = winsvc.GetAppPath(); err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(filepath.Dir(appPath)); err != nil {
		log.Fatal(err)
	}
}

func initLog(output string) {
	switch {
	case output == "stdout":
		log.SetOutput(os.Stdout)
	case output == "stderr":
		log.SetOutput(os.Stderr)
	case len(output) != 0:
		log.SetOutput(&lumberjack.Logger{
			Filename:   output,
			MaxSize:    config.Config.Log.MaxSize,
			MaxAge:     config.Config.Log.MaxAge,
			MaxBackups: config.Config.Log.MaxBackups,
			LocalTime:  config.Config.Log.LocalTime,
			Compress:   config.Config.Log.Compress,
		})
	default:
		log.SetOutput(os.Stdout)
	}
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(config.Version)
		os.Exit(0)
	}

	// init configs
	if err := config.InitConfig(*configDir, *debugMode, *testMode, *interval, *inputFilters); err != nil {
		log.Fatalln("F! failed to init config:", err)
	}

	doOSsvc()
	printEnv()

	initWriters()

	go api.Start()
	go heartbeat.Work()

	ag, err := agent.NewAgent()
	if err != nil {
		fmt.Println("F! failed to init agent:", err)
		os.Exit(-1)
	}
	runAgent(ag)
}

func initWriters() {
	if err := writer.InitWriters(); err != nil {
		log.Fatalln("F! failed to init writer:", err)
	}
}

func handleSignal(ag *agent.Agent) {

	sc := make(chan os.Signal, 1)
	// syscall.SIGUSR2 == 0xc , not available on windows
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGPIPE)

EXIT:
	for {
		sig := <-sc
		log.Println("I! received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			break EXIT
		case syscall.SIGHUP:
			ag.Reload()
		case syscall.SIGPIPE:
			// https://pkg.go.dev/os/signal#hdr-SIGPIPE
			// do nothing
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
