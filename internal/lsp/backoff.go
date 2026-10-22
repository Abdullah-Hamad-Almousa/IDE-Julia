package lsp

import (
	"context"
	"fmt"
	"time"
)

// Clock is the time source of the client.
//
// The interface exists so that the restart supervisor can be driven
// deterministically from a test: the test observes the delays that the
// supervisor asks for instead of waiting for them.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Sleep blocks until d has passed or until the context is done. It
	// returns the error of the context when the wait was interrupted.
	Sleep(ctx context.Context, d time.Duration) error
}

// SystemClock is the Clock backed by the time package.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep blocks until d has passed or until the context is done.
func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// BackoffPolicy describes how the client restarts a language server that
// died.
//
// The delay of the first attempt is InitialDelay. Every further attempt
// waits Multiplier times as long, up to MaxDelay. The client gives up after
// MaxAttempts attempts.
type BackoffPolicy struct {
	// InitialDelay is the delay before the first restart.
	InitialDelay time.Duration
	// Multiplier grows the delay after every attempt. A value of one keeps
	// the delay constant.
	Multiplier float64
	// MaxDelay caps the delay. A value of zero does not cap it.
	MaxDelay time.Duration
	// MaxAttempts is the number of restarts. A value of zero keeps the
	// client from restarting the server at all.
	MaxAttempts int
}

// DefaultBackoffPolicy returns the restart policy of the client.
//
// The policy waits a second before the first restart and doubles the delay
// every time, up to half a minute. It gives up after five attempts.
func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{
		InitialDelay: time.Second,
		Multiplier:   2,
		MaxDelay:     30 * time.Second,
		MaxAttempts:  5,
	}
}

// Delay returns the delay of the given attempt.
//
// The attempt is one based: the first attempt waits for InitialDelay. An
// attempt of zero or less waits for nothing.
func (p BackoffPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	delay := float64(p.InitialDelay)
	for step := 1; step < attempt; step++ {
		delay *= p.Multiplier
		if p.MaxDelay > 0 && delay >= float64(p.MaxDelay) {
			return p.MaxDelay
		}
	}
	if p.MaxDelay > 0 && delay > float64(p.MaxDelay) {
		return p.MaxDelay
	}
	return time.Duration(delay)
}

// String renders the policy for the status of the server.
func (p BackoffPolicy) String() string {
	return fmt.Sprintf("initial %s, multiplier %.2f, maximum %s, attempts %d",
		p.InitialDelay, p.Multiplier, p.MaxDelay, p.MaxAttempts)
}
