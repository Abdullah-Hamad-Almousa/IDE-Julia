package lsp

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock records the delays that the restart supervisor asks for.
//
// The clock blocks in Sleep until the test releases it, so that the test can
// observe the state of the client while the supervisor waits.
type fakeClock struct {
	mu      sync.Mutex
	delays  []time.Duration
	release chan struct{}
	changed chan struct{}
}

// newFakeClock returns a clock that blocks until it is released.
func newFakeClock() *fakeClock {
	return &fakeClock{
		release: make(chan struct{}),
		changed: make(chan struct{}, 16),
	}
}

// Now returns the current time.
func (c *fakeClock) Now() time.Time { return time.Time{} }

// Sleep records the delay and waits for the release of the test.
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.delays = append(c.delays, d)
	c.mu.Unlock()
	select {
	case c.changed <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
	}
}

// waitForDelay waits for the next delay of the supervisor.
func (c *fakeClock) waitForDelay(t *testing.T) time.Duration {
	t.Helper()
	select {
	case <-c.changed:
	case <-time.After(15 * time.Second):
		t.Fatal("the restart supervisor did not ask for a delay")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.delays) == 0 {
		t.Fatal("the restart supervisor did not ask for a delay")
	}
	return c.delays[len(c.delays)-1]
}

// delaysOf returns the delays that the supervisor asked for.
func (c *fakeClock) delaysOf() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]time.Duration, len(c.delays))
	copy(result, c.delays)
	return result
}

// TestClientRestartsTheServerAfterACrash kills the fake server and checks
// that the client starts it again.
//
// The test also checks the delays of the backoff policy: the first restart
// waits for the initial delay, the second one for the initial delay times
// the multiplier.
func TestClientRestartsTheServerAfterACrash(t *testing.T) {
	directory := t.TempDir()
	command, logPath := fakeServerCommand(t, directory)
	clock := newFakeClock()
	client := New(Options{
		Command: command,
		Backoff: BackoffPolicy{
			InitialDelay: 10 * time.Millisecond,
			Multiplier:   3,
			MaxDelay:     time.Second,
			MaxAttempts:  3,
		},
		Clock: clock,
	})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	waitForTrace(t, logPath, "spawn", 1)

	killServer(t, client)
	if got, want := clock.waitForDelay(t), 10*time.Millisecond; got != want {
		t.Errorf("the delay of the first restart = %s, want %s", got, want)
	}
	if got, want := client.State(), StateRestarting; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}

	close(clock.release)
	waitForTrace(t, logPath, "spawn", 2)
	if got, want := client.State(), StateRunning; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}
	killServer(t, client)
	if got, want := clock.waitForDelay(t), 30*time.Millisecond; got != want {
		t.Errorf("the delay of the second restart = %s, want %s", got, want)
	}
	waitForTrace(t, logPath, "spawn", 3)

	delays := clock.delaysOf()
	if len(delays) != 2 {
		t.Fatalf("the supervisor asked for %d delays, want 2: %v", len(delays), delays)
	}
	if got, want := delays[0], 10*time.Millisecond; got != want {
		t.Errorf("the first delay = %s, want %s", got, want)
	}
	if got, want := delays[1], 30*time.Millisecond; got != want {
		t.Errorf("the second delay = %s, want %s", got, want)
	}
}

// TestClientOpensTheDocumentsAgainAfterACrash checks that the client tells
// the restarted server about the documents of the editor.
func TestClientOpensTheDocumentsAgainAfterACrash(t *testing.T) {
	directory := t.TempDir()
	command, logPath := fakeServerCommand(t, directory)
	clock := newFakeClock()
	client := New(Options{
		Command: command,
		Backoff: BackoffPolicy{
			InitialDelay: time.Millisecond,
			Multiplier:   2,
			MaxDelay:     time.Second,
			MaxAttempts:  3,
		},
		Clock: clock,
	})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	uri := "file:///workspace/main.jl"
	if err := client.DidOpen(ctx, uri, "julia", "f(x) = x + 1\n", 1); err != nil {
		t.Fatalf("DidOpen() error = %v", err)
	}
	waitForTrace(t, logPath, "message textDocument/didOpen", 1)

	killServer(t, client)
	clock.waitForDelay(t)
	close(clock.release)
	waitForTrace(t, logPath, "message textDocument/didOpen", 2)
}

func TestClientOpensDocumentsAddedWhileStoppedOnStart(t *testing.T) {
	directory := t.TempDir()
	command, logPath := fakeServerCommand(t, directory)
	client := New(Options{Command: command})
	uri := "file:///workspace/queued.jl"
	if err := client.DidOpen(context.Background(), uri, "julia", "queued() = 42\n", 3); err != nil {
		t.Fatalf("DidOpen() while stopped error = %v", err)
	}
	if got := client.State(); got != StateStopped {
		t.Fatalf("state after queuing document = %q, want stopped", got)
	}
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	trace := waitForTrace(t, logPath, "message textDocument/didOpen", 1)
	if !containsLine(trace, "message textDocument/didOpen") {
		t.Fatalf("server did not receive queued document: %v", trace)
	}
	if client.State() != StateRunning {
		t.Fatalf("state = %q, want running", client.State())
	}
}

// TestClientGivesUpAfterTheLastAttempt checks that the client stops the
// restarts when the policy is exhausted.
func TestClientGivesUpAfterTheLastAttempt(t *testing.T) {
	directory := t.TempDir()
	command, logPath := fakeServerCommand(t, directory)
	clock := newFakeClock()
	client := New(Options{
		Command: command,
		Backoff: BackoffPolicy{
			InitialDelay: time.Millisecond,
			Multiplier:   2,
			MaxDelay:     time.Second,
			MaxAttempts:  1,
		},
		Clock: clock,
	})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	killServer(t, client)
	clock.waitForDelay(t)
	close(clock.release)
	waitForTrace(t, logPath, "spawn", 2)

	killServer(t, client)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if client.State() == StateStopped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got, want := client.State(), StateStopped; got != want {
		t.Errorf("State() = %q, want %q", got, want)
	}
	if client.Status().LastError == "" {
		t.Error("the status does not describe the last failure")
	}
}

// killServer kills the process of the fake language server.
func killServer(t *testing.T, client *Client) {
	t.Helper()
	proc := client.currentProcess()
	if proc == nil || proc.cmd.Process == nil {
		t.Fatal("the language server does not run")
	}
	if err := proc.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill the language server: %v", err)
	}
}
