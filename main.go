package main

import (
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/chai2010/winsvc"
	"github.com/kardianos/service"
	"github.com/toolkits/pkg/net/tcpx"
	"github.com/toolkits/pkg/runner"
	"gopkg.in/natefinch/lumberjack.v2"

	"flashcat.cloud/categraf/agent"
	agentInstall "flashcat.cloud/categraf/agent/install"
	agentUpdate "flashcat.cloud/categraf/agent/update"
	"flashcat.cloud/categraf/api"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/heartbeat"
	"flashcat.cloud/categraf/pkg/osx"
	"flashcat.cloud/categraf/writer"
)

var (
	appPath      string
	configDir    = flag.String("configs", osx.GetEnv("CATEGRAF_CONFIGS", "conf"), "Specify configuration directory.(env:CATEGRAF_CONFIGS)")
	debugMode    = flag.Bool("debug", false, "Is debug mode?")
	debugLevel   = flag.Int("debug-level", 0, "debug level")
	testMode     = flag.Bool("test", false, "Is test mode? print metrics to stdout")
	interval     = flag.Int64("interval", 0, "Global interval(unit:Second)")
	showVersion  = flag.Bool("version", false, "Show version.")
	inputFilters = flag.String("inputs", "", "e.g. cpu:mem:system")
	install      = flag.Bool("install", false, "Install categraf service")
	remove       = flag.Bool("remove", false, "Remove categraf service")
	start        = flag.Bool("start", false, "Start categraf service")
	stop         = flag.Bool("stop", false, "Stop categraf service")
	status       = flag.Bool("status", false, "Show categraf service status")
	update       = flag.Bool("update", false, "Update categraf binary")
	updateFile   = flag.String("update_url", "", "new version for categraf to download")
	userMode     = flag.Bool("user", false, "Install categraf service with user mode")
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
	if *install || *remove || *start || *stop || *status || *update {
		err := serviceProcess()
		if err != nil {
			log.Println("E!", err)
		}
		return
	}

	// init configs
	if err := config.InitConfig(*configDir, *debugLevel, *debugMode, *testMode, *interval, *inputFilters); err != nil {
		log.Fatalln("F! failed to init config:", err)
	}

	doOSsvc()
	printEnv()

	initWriters()

	go api.Start()
	go heartbeat.Work()

	tcpx.WaitHosts()
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
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			log.Println("I! received signal:", sig.String())
			break EXIT
		case syscall.SIGHUP:
			log.Println("I! received signal:", sig.String())
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

type program struct{}

func (p *program) Start(s service.Service) error {
	return nil
}

func (p *program) Stop(s service.Service) error {
	return nil
}

func serviceProcess() error {
	svcConfig := agentInstall.ServiceConfig(*userMode)
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Println("generate categraf service error " + err.Error())
		return nil
	}

	if *stop {
		if sts, err := s.Status(); err != nil {
			log.Println("W! show categraf service status failed:", err)
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! categraf service status: running")
			case service.StatusStopped:
				log.Println("I! categraf service status: stopped")
			default:
				log.Println("I! categraf service status: unknown")
			}
		}
		if err := s.Stop(); err != nil {
			log.Println("E! stop categraf service failed:", err)
		} else {
			log.Println("I! stop categraf service ok")
		}
		return nil
	}

	if *remove {
		if sts, err := s.Status(); err != nil {
			log.Println("W! show categraf service status failed:", err)
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! categraf service status: running")
			case service.StatusStopped:
				log.Println("I! categraf service status: stopped")
			default:
				log.Println("I! categraf service status: unknown")
			}
		}
		if err := s.Stop(); err != nil {
			log.Println("W! stop categraf service failed:", err)
		} else {
			log.Println("I! stop categraf service ok")
		}
		if err := s.Uninstall(); err != nil {
			log.Println("E! remove categraf service failed:", err)
		} else {
			log.Println("I! remove categraf service ok")
		}
		return nil
	}

	if *install {
		if sts, err := s.Status(); err != nil {
			log.Println("W! show categraf service status failed:", err)
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! categraf service status: running")
			case service.StatusStopped:
				log.Println("I! categraf service status: stopped")
			default:
				log.Println("I! categraf service status: unknown")
			}
		}
		if err := s.Install(); err != nil {
			log.Println("E! install categraf service failed:", err)
		} else {
			log.Println("I! install categraf service ok")
		}
		return nil
	}

	if *start {
		if sts, err := s.Status(); err != nil {
			log.Println("W! show categraf service status failed:", err)
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! categraf service status: running")
			case service.StatusStopped:
				log.Println("I! categraf service status: stopped")
			default:
				log.Println("I! categraf service status: unknown")
			}
		}
		if err := s.Start(); err != nil {
			log.Println("E! start categraf service failed:", err)
		} else {
			log.Println("I! start categraf service ok")
		}
		return nil
	}
	if *status {
		if sts, err := s.Status(); err != nil {
			log.Println("E! show categraf service status failed:", err)
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! show categraf service status: running")
			case service.StatusStopped:
				log.Println("I! show categraf service status: stopped")
			default:
				log.Println("I! show categraf service status: unknown")
			}
		}

		return nil
	}
	if *update {
		if *updateFile == "" {
			return fmt.Errorf("please input update_url")
		}
		if sts, err := s.Status(); err != nil {
			if strings.Contains(err.Error(), "not installed") {
				log.Println("E! update only support mode that running in service mode")
			}
			return nil
		} else {
			switch sts {
			case service.StatusRunning:
				log.Println("I! categraf service status: running, version:", config.Version)
			case service.StatusStopped:
				log.Println("I! categraf service status: stopped, version:", config.Version)
			default:
				log.Println("I! categraf service status: unknown, version:", config.Version)
			}
		}
		err := agentUpdate.Update(*updateFile)
		if err != nil {
			log.Println("E! update categraf failed:", err)
			return nil
		}
		err = s.Restart()
		if err != nil {
			log.Println("E! restart categraf failed:", err)
			return nil
		}
		log.Println("I! update categraf success")
	}
	return nil
}
