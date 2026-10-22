package vfs

import (
	"sync"
	"time"
)

// fakeClock is a deterministic Clock for the tests.
//
// The clock owns the timers and the tickers it hands out, so a test can move
// the time forward and let the scheduled callbacks run in a defined order.
type fakeClock struct {
	mutex   sync.Mutex
	now     time.Time
	timers  []*fakeTimer
	tickers []*fakeTicker
}

// newFakeClock returns a clock that starts at a fixed point in time.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.October, 22, 12, 0, 0, 0, time.UTC)}
}

// Now returns the current time of the clock.
func (c *fakeClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.now
}

// AfterFunc schedules f to run after d.
func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	timer := &fakeTimer{clock: c, deadline: c.now.Add(d), fn: f}
	c.timers = append(c.timers, timer)
	return timer
}

// NewTicker returns a ticker that delivers a tick every d.
func (c *fakeClock) NewTicker(d time.Duration) Ticker {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ticker := &fakeTicker{clock: c, interval: d, ch: make(chan time.Time, 64)}
	c.tickers = append(c.tickers, ticker)
	return ticker
}

// Advance moves the clock forward by d and runs the callbacks that are due.
//
// The callbacks run in the goroutine of the caller, so a test observes their
// effect without waiting.
func (c *fakeClock) Advance(d time.Duration) {
	c.mutex.Lock()
	c.now = c.now.Add(d)
	now := c.now

	var due []*fakeTimer
	pending := c.timers[:0]
	for _, timer := range c.timers {
		if !timer.stopped && !timer.deadline.After(now) {
			due = append(due, timer)
			continue
		}
		pending = append(pending, timer)
	}
	c.timers = pending

	for _, ticker := range c.tickers {
		if ticker.stopped {
			continue
		}
		select {
		case ticker.ch <- now:
		default:
		}
	}
	c.mutex.Unlock()

	for _, timer := range due {
		timer.fn()
	}
}

// tickerCount returns the number of tickers the clock handed out.
func (c *fakeClock) tickerCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.tickers)
}

// fakeTimer is a timer of the fake clock.
type fakeTimer struct {
	clock    *fakeClock
	deadline time.Time
	fn       func()
	stopped  bool
}

// Stop cancels the timer.
func (t *fakeTimer) Stop() bool {
	t.clock.mutex.Lock()
	defer t.clock.mutex.Unlock()
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

// fakeTicker is a ticker of the fake clock.
type fakeTicker struct {
	clock    *fakeClock
	interval time.Duration
	ch       chan time.Time
	stopped  bool
}

// C returns the channel the ticks are delivered on.
func (t *fakeTicker) C() <-chan time.Time { return t.ch }

// Stop stops the ticker.
func (t *fakeTicker) Stop() {
	t.clock.mutex.Lock()
	defer t.clock.mutex.Unlock()
	t.stopped = true
}
