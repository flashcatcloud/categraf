//go:build !no_logs

package util

import (
	"log"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var logsPipelinePanicTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "categraf_logs_pipeline_panic_total",
		Help: "Total number of recovered panics in the logs pipeline.",
	},
	[]string{"component"},
)

func init() {
	prometheus.MustRegister(logsPipelinePanicTotal)
}

// SafeGo runs fn in a new goroutine with panic recovery.
// On panic it increments the panic counter for the given component,
// logs the stack trace, and calls onPanic if non-nil.
func SafeGo(component string, fn func(), onPanic func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logsPipelinePanicTotal.WithLabelValues(component).Inc()
				log.Printf("E! [%s] panic recovered: %v\n%s", component, r, debug.Stack())
				if onPanic != nil {
					onPanic()
				}
			}
		}()
		fn()
	}()
}

// SafeGoWithRestart runs fn in a new goroutine with panic recovery and
// automatic restart after a backoff delay.
// If stopChan is not nil, it will abort restart if stopChan is closed.
// onDone is called when fn exits naturally, or when restarts are aborted.
func SafeGoWithRestart(component string, fn func(), backoff time.Duration, stopChan chan struct{}, onDone func()) {
	go func() {
		if onDone != nil {
			defer onDone()
		}
		for {
			panicked := true
			ch := make(chan struct{})
			SafeGo(component, func() {
				defer close(ch)
				fn()
				panicked = false
			}, nil)
			<-ch // wait for fn to finish or panic

			if !panicked {
				return // exited naturally
			}

			log.Printf("W! [%s] restarting after %v backoff", component, backoff)
			if stopChan != nil {
				select {
				case <-time.After(backoff):
				case <-stopChan:
					log.Printf("I! [%s] shutdown signal received, aborting restart", component)
					return
				}
			} else {
				time.Sleep(backoff)
			}
		}
	}()
}
