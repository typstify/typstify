package utils

import (
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultDebounce    = time.Millisecond * 100
	defaultMaxInterval = time.Millisecond * 200
)

// Debouncer implemented the debounce pattern with a maximum execution delay.
// Unlike a standard debouncer that resets the timer on every call, potentially
// delaying execution indefinitely, this implementation uses MaxInterval to ensure
// callbacks to be executed at least once during the period of a sustained burst
// invocations.
type Debouncer struct {
	timer   *time.Timer
	lastRun time.Time
	pending atomic.Bool
	mu      sync.Mutex

	Debounce    time.Duration
	MaxInterval time.Duration
}

func (d *Debouncer) Run(runner func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Debounce == 0 {
		d.Debounce = defaultDebounce
	}
	if d.MaxInterval == 0 {
		d.MaxInterval = defaultMaxInterval
	}

	now := time.Now()

	// If too long since last run, run immediately
	if now.Sub(d.lastRun) >= d.MaxInterval {
		d.lastRun = now
		if d.timer != nil {
			d.timer.Stop()
		}
		d.pending.Store(false)
		go runner()
		return
	}

	// Otherwise debounce
	d.pending.Store(true)

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.Debounce, func() {
		if d.pending.CompareAndSwap(true, false) {
			d.mu.Lock()
			d.lastRun = time.Now()
			d.mu.Unlock()

			go runner()
		}
	})

}

func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
}
