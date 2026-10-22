package vfs

import "time"

// Clock is the time source of the virtual file system.
//
// The interface exists so that the debouncer and the polling watcher can be
// driven deterministically from a test, without sleeping and without a real
// timer.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// AfterFunc schedules f to run after d and returns a timer that can
	// cancel the call.
	AfterFunc(d time.Duration, f func()) Timer
	// NewTicker returns a ticker that delivers a tick every d.
	NewTicker(d time.Duration) Ticker
}

// Timer is a scheduled callback.
type Timer interface {
	// Stop cancels the timer. It reports whether the call was prevented.
	Stop() bool
}

// Ticker delivers periodic ticks.
type Ticker interface {
	// C returns the channel the ticks are delivered on.
	C() <-chan time.Time
	// Stop stops the ticker.
	Stop()
}

// SystemClock is the Clock backed by the time package.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// AfterFunc schedules f to run after d.
func (SystemClock) AfterFunc(d time.Duration, f func()) Timer {
	return systemTimer{timer: time.AfterFunc(d, f)}
}

// NewTicker returns a ticker that delivers a tick every d.
func (SystemClock) NewTicker(d time.Duration) Ticker {
	return systemTicker{ticker: time.NewTicker(d)}
}

// systemTimer adapts a time.Timer to the Timer interface.
type systemTimer struct {
	timer *time.Timer
}

// Stop cancels the timer.
func (t systemTimer) Stop() bool { return t.timer.Stop() }

// systemTicker adapts a time.Ticker to the Ticker interface.
type systemTicker struct {
	ticker *time.Ticker
}

// C returns the channel the ticks are delivered on.
func (t systemTicker) C() <-chan time.Time { return t.ticker.C }

// Stop stops the ticker.
func (t systemTicker) Stop() { t.ticker.Stop() }
