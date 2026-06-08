package reloadwatcher

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	target  string
	watcher *fsnotify.Watcher
	done    chan struct{}
	once    sync.Once
}

func Start(target string, debounce time.Duration, onChange func()) (*Watcher, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		target:  filepath.Clean(absTarget),
		watcher: fsw,
		done:    make(chan struct{}),
	}
	if err := fsw.Add(filepath.Dir(absTarget)); err != nil {
		fsw.Close()
		return nil, err
	}

	go w.run(debounce, onChange)
	return w, nil
}

func (w *Watcher) Close() error {
	var err error
	w.once.Do(func() {
		close(w.done)
		err = w.watcher.Close()
	})
	return err
}

func (w *Watcher) run(debounce time.Duration, onChange func()) {
	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	defer stopTimer()

	resetTimer := func() {
		if debounce <= 0 {
			onChange()
			return
		}
		if timer == nil {
			timer = time.NewTimer(debounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
	}

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if w.isTargetEvent(event) {
				resetTimer()
			}
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		case <-timerC:
			timer = nil
			timerC = nil
			onChange()
		case <-w.done:
			return
		}
	}
}

func (w *Watcher) isTargetEvent(event fsnotify.Event) bool {
	if filepath.Clean(event.Name) != w.target {
		return false
	}
	return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0
}
