package main

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/chai2010/winsvc"
	"github.com/kardianos/service"
	"github.com/toolkits/pkg/net/tcpx"
	"github.com/toolkits/pkg/runner"
	"k8s.io/klog/v2"

	"flashcat.cloud/categraf/agent"
	agentInstall "flashcat.cloud/categraf/agent/install"
	agentUpdate "flashcat.cloud/categraf/agent/update"
	"flashcat.cloud/categraf/api"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/heartbeat"
	"flashcat.cloud/categraf/pkg/logging"
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
	logging.RegisterFlags(flag.CommandLine)

	// change to current dir
	var err error
	if appPath, err = winsvc.GetAppPath(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(filepath.Dir(appPath)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initLog(output string) {
	if output == "" {
		output = config.Config.Log.FileName
		if config.Config.Log.FileName == "stdout" || config.Config.Log.FileName == "stderr" || config.Config.Log.FileName == "" {
			if runtime.GOOS == "windows" && !winsvc.IsAnInteractiveSession() {
				output = "categraf.log"
			}
		}
	}

	if err := logging.Configure(
		output,
		config.Config.Log.MaxSize,
		config.Config.Log.MaxAge,
		config.Config.Log.MaxBackups,
		config.Config.Log.LocalTime,
		config.Config.Log.Compress,
		config.Config.DebugMode,
		config.Config.DebugLevel,
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initServiceCommandLog() {
	if err := logging.Configure("stderr", 0, 0, 0, false, false, *debugMode, *debugLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(config.Version)
		os.Exit(0)
	}

	if *install || *remove || *start || *stop || *status || *update {
		initServiceCommandLog()
		defer logging.Sync()

		if err := serviceProcess(); err != nil {
			klog.ErrorS(err, "service command failed")
		}
		return
	}

	// init configs
	if err := config.InitConfig(*configDir, *debugLevel, *debugMode, *testMode, *interval, *inputFilters); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init config: %v\n", err)
		os.Exit(1)
	}

	initLog("")
	defer logging.Sync()

	doOSsvc()
	printEnv()

	initWriters()

	go api.Start()
	go heartbeat.Work()

	tcpx.WaitHosts()
	ag, err := agent.NewAgent()
	if err != nil {
		klog.ErrorS(err, "failed to init agent")
		logging.Sync()
		os.Exit(1)
	}
	runAgent(ag)
}

func initWriters() {
	if err := writer.InitWriters(); err != nil {
		klog.ErrorS(err, "failed to init writer")
		logging.Sync()
		os.Exit(1)
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
			klog.InfoS("received signal", "signal", sig.String())
			break EXIT
		case syscall.SIGHUP:
			klog.InfoS("received signal", "signal", sig.String())
			ag.Reload()
		case syscall.SIGPIPE:
			// https://pkg.go.dev/os/signal#hdr-SIGPIPE
			// do nothing
		}
	}

	ag.Stop()
	logging.Sync()
	klog.InfoS("exited")
}

func printEnv() {
	runner.Init()
	klog.InfoS("runner environment", "binarydir", runner.Cwd, "hostname", runner.Hostname, "fd_limits", runner.FdLimits(), "vm_limits", runner.VMLimits())
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
		klog.ErrorS(err, "generate categraf service error")
		return nil
	}

	if *stop {
		if sts, err := s.Status(); err != nil {
			klog.Warningf("show categraf service status failed: %v", err)
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("categraf service status", "status", "running")
			case service.StatusStopped:
				klog.InfoS("categraf service status", "status", "stopped")
			default:
				klog.InfoS("categraf service status", "status", "unknown")
			}
		}
		if err := s.Stop(); err != nil {
			klog.ErrorS(err, "stop categraf service failed")
		} else {
			klog.InfoS("stop categraf service ok")
		}
		return nil
	}

	if *remove {
		if sts, err := s.Status(); err != nil {
			klog.Warningf("show categraf service status failed: %v", err)
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("categraf service status", "status", "running")
			case service.StatusStopped:
				klog.InfoS("categraf service status", "status", "stopped")
			default:
				klog.InfoS("categraf service status", "status", "unknown")
			}
		}
		if err := s.Stop(); err != nil {
			klog.ErrorS(err, "stop categraf service failed")
		} else {
			klog.InfoS("stop categraf service ok")
		}
		if err := s.Uninstall(); err != nil {
			klog.ErrorS(err, "remove categraf service failed")
		} else {
			klog.InfoS("remove categraf service ok")
		}
		return nil
	}

	if *install {
		if sts, err := s.Status(); err != nil {
			klog.Warningf("show categraf service status failed: %v", err)
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("categraf service status", "status", "running")
			case service.StatusStopped:
				klog.InfoS("categraf service status", "status", "stopped")
			default:
				klog.InfoS("categraf service status", "status", "unknown")
			}
		}
		if err := s.Install(); err != nil {
			klog.ErrorS(err, "install categraf service failed")
		} else {
			klog.InfoS("install categraf service ok")
		}
		return nil
	}

	if *start {
		if sts, err := s.Status(); err != nil {
			klog.Warningf("show categraf service status failed: %v", err)
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("categraf service status", "status", "running")
			case service.StatusStopped:
				klog.InfoS("categraf service status", "status", "stopped")
			default:
				klog.InfoS("categraf service status", "status", "unknown")
			}
		}
		if err := s.Start(); err != nil {
			klog.ErrorS(err, "start categraf service failed")
		} else {
			klog.InfoS("start categraf service ok")
		}
		return nil
	}
	if *status {
		if sts, err := s.Status(); err != nil {
			klog.ErrorS(err, "show categraf service status failed")
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("show categraf service status", "status", "running")
			case service.StatusStopped:
				klog.InfoS("show categraf service status", "status", "stopped")
			default:
				klog.InfoS("show categraf service status", "status", "unknown")
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
				klog.Warningf("update only support mode that running in service mode")
			}
			return nil
		} else {
			switch sts {
			case service.StatusRunning:
				klog.InfoS("categraf service status", "status", "running", "version", config.Version)
			case service.StatusStopped:
				klog.InfoS("categraf service status", "status", "stopped", "version", config.Version)
			default:
				klog.InfoS("categraf service status", "status", "unknown", "version", config.Version)
			}
		}
		err := agentUpdate.Update(*updateFile)
		if err != nil {
			klog.ErrorS(err, "update categraf failed")
			return nil
		}
		err = s.Restart()
		if err != nil {
			klog.ErrorS(err, "restart categraf failed")
			return nil
		}
		klog.InfoS("update categraf success")
	}
	return nil
}
