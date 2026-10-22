package vfs

import (
	"sync"
	"time"
)

// Change is a debounced notification about the paths that changed.
type Change struct {
	// Paths holds the paths that changed during the coalescing window, in the
	// order they were first reported.
	Paths []string
	// At is the time the notification was emitted.
	At time.Time
}

// Debouncer coalesces a burst of notifications into a single change event.
//
// Every call of Notify restarts the timer, so a burst of file-system events
// produces exactly one change event once the burst is over. The debounce
// interval is a parameter, not a constant: a caller that needs a different
// trade-off between latency and the number of notifications passes a
// different interval.
type Debouncer struct {
	clock    Clock
	interval time.Duration
	changes  chan Change
	done     chan struct{}

	mutex   sync.Mutex
	pending []string
	seen    map[string]bool
	timer   Timer
	closed  bool
}

// NewDebouncer returns a debouncer that uses the given interval.
//
// The clock is the time source of the debouncer; a test can pass a fake
// clock to drive the debouncer deterministically.
func NewDebouncer(interval time.Duration, clock Clock) *Debouncer {
	if interval <= 0 {
		interval = time.Millisecond
	}
	return &Debouncer{
		clock:    clock,
		interval: interval,
		changes:  make(chan Change, eventBufferSize),
		done:     make(chan struct{}),
		seen:     make(map[string]bool),
	}
}

// Interval returns the debounce interval of the debouncer.
func (d *Debouncer) Interval() time.Duration { return d.interval }

// Notify reports that the path changed.
//
// The path is added to the pending set of the current window. A path that is
// reported twice in the same window is reported once.
func (d *Debouncer) Notify(path string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed {
		return
	}
	if !d.seen[path] {
		d.seen[path] = true
		d.pending = append(d.pending, path)
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = d.clock.AfterFunc(d.interval, d.flush)
}

// Changes returns the channel the coalesced notifications are delivered on.
//
// The channel is not closed when the debouncer is closed: a notification that
// is already in flight would race with the close. A consumer selects on the
// channel and on the Done channel.
func (d *Debouncer) Changes() <-chan Change { return d.changes }

// Done returns the channel that is closed when the debouncer is closed.
func (d *Debouncer) Done() <-chan struct{} { return d.done }

// Close stops the debouncer. The pending notifications are dropped.
func (d *Debouncer) Close() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	close(d.done)
	return nil
}

// flush emits the pending notifications as a single change event.
func (d *Debouncer) flush() {
	d.mutex.Lock()
	if d.closed {
		d.mutex.Unlock()
		return
	}
	paths := d.pending
	d.pending = nil
	d.seen = make(map[string]bool)
	d.timer = nil
	d.mutex.Unlock()

	if len(paths) == 0 {
		return
	}
	change := Change{Paths: paths, At: d.clock.Now()}
	select {
	case d.changes <- change:
	case <-d.done:
	}
}

// Pipeline forwards the events of a store into a debounced change stream.
//
// It is the save pipeline of the engine: a burst of edits produces a single
// change notification, so the expensive work behind it (a reparse, an index
// update, a language-server notification) runs once per burst.
type Pipeline struct {
	store     *Store
	debouncer *Debouncer
	sub       *Subscription
}

// NewPipeline returns a pipeline that debounces the events of the store.
//
// The interval is the debounce interval; the clock is the time source of the
// debouncer.
func NewPipeline(store *Store, interval time.Duration, clock Clock) *Pipeline {
	pipeline := &Pipeline{
		store:     store,
		debouncer: NewDebouncer(interval, clock),
	}
	pipeline.sub = store.Subscribe()
	go pipeline.run()
	return pipeline
}

// Changes returns the channel the coalesced notifications are delivered on.
func (p *Pipeline) Changes() <-chan Change { return p.debouncer.Changes() }

// Done returns the channel that is closed when the pipeline is closed.
func (p *Pipeline) Done() <-chan struct{} { return p.debouncer.Done() }

// Close stops the pipeline and releases its subscription.
func (p *Pipeline) Close() error {
	p.sub.Close()
	return p.debouncer.Close()
}

// run forwards the write and save events of the store to the debouncer.
func (p *Pipeline) run() {
	for {
		select {
		case <-p.sub.Done():
			return
		case event, ok := <-p.sub.Events():
			if !ok {
				return
			}
			if event.Kind == EventWrite || event.Kind == EventSave {
				p.debouncer.Notify(event.Path)
			}
		}
	}
}
