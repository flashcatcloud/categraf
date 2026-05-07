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

// SafeGoWithRestart runs fn in a goroutine with panic recovery and
// automatic restart after a backoff delay.
func SafeGoWithRestart(component string, fn func(), backoff time.Duration) {
	SafeGo(component, fn, func() {
		log.Printf("W! [%s] restarting after %v backoff", component, backoff)
		time.Sleep(backoff)
		SafeGoWithRestart(component, fn, backoff)
	})
}
