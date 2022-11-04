package logstream

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type ReadDeadliner interface {
	SetReadDeadline(t time.Time) error
}

func SetReadDeadlineOnDone(ctx context.Context, d ReadDeadliner) {
	go func() {
		<-ctx.Done()
		log.Println("cancelled, setting read deadline to interrupt read")
		if err := d.SetReadDeadline(time.Now()); err != nil {
			log.Println(err)
		}
	}()
}

func IsEndOrCancel(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	if os.IsTimeout(err) {
		return true
	}
	// https://github.com/golang/go/issues/4373
	if strings.Contains(err.Error(), "use of closed network connection") {
		return true
	}
	return false
}
