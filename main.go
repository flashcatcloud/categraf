package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/house"
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
	showVersion  = flag.Bool("version", false, "Show version.")
	inputFilters = flag.String("inputs", "", "e.g. cpu:mem:system")

	flagWinSvcName      = flag.String("win-service-name", "categraf", "Set windows service name")
	flagWinSvcDesc      = flag.String("win-service-desc", "Categraf", "Set windows service description")
	flagWinSvcInstall   = flag.Bool("win-service-install", false, "Install windows service")
	flagWinSvcUninstall = flag.Bool("win-service-uninstall", false, "Uninstall windows service")
	flagWinSvcStart     = flag.Bool("win-service-start", false, "Start windows service")
	flagWinSvcStop      = flag.Bool("win-service-stop", false, "Stop windows service")
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

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(config.Version)
		os.Exit(0)
	}

	doWinsvc()
	printEnv()

	// init configs
	if err := config.InitConfig(*configDir, *debugMode, *testMode); err != nil {
		log.Fatalln("F! failed to init config:", err)
	}

	initWriters()

	ag := agent.NewAgent(parseFilter(*inputFilters))
	runAgent(ag)
}

func initWriters() {
	if err := writer.InitWriters(); err != nil {
		log.Fatalln("F! failed to init writer:", err)
	}

	if err := house.InitMetricsHouse(); err != nil {
		log.Fatalln("F! failed to init metricshouse:", err)
	}
}

func handleSignal(ag *agent.Agent) {
	sc := make(chan os.Signal, 1)
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

func parseFilter(filterStr string) map[string]struct{} {
	filters := strings.Split(filterStr, ":")
	filtermap := make(map[string]struct{})
	for i := 0; i < len(filters); i++ {
		if strings.TrimSpace(filters[i]) == "" {
			continue
		}
		filtermap[filters[i]] = struct{}{}
	}
	return filtermap
}

func doWinsvc() {
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
