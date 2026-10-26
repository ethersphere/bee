// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// stalledWriter blocks every Write until it is released,
// mimicking a log consumer that stopped reading.
type stalledWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	buf     bytes.Buffer
}

func newStalledWriter() *stalledWriter {
	return &stalledWriter{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (s *stalledWriter) Write(p []byte) (int, error) {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-s.release
	return s.buf.Write(p)
}

func (s *stalledWriter) unblock() {
	s.once.Do(func() { close(s.release) })
}

// slowWriter delays every Write, mimicking a log
// consumer that is slow but keeps on reading.
type slowWriter struct {
	delay time.Duration
	buf   bytes.Buffer
}

func (s *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(s.delay)
	return s.buf.Write(p)
}

// failingWriter fails every Write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// newNonBlockingWriter returns a NonBlockingWriter with a short stall
// timeout, whose dropped messages notice is logged without pinning
// the defaults, see newLogger.
func newNonBlockingWriter(w io.Writer, size int) *NonBlockingWriter {
	nb := NewNonBlockingWriter(w, size)
	nb.stallTimeout = 20 * time.Millisecond
	l, _ := newLogger(WithVerbosity(VerbosityWarning))
	l.(*logger).sink = w
	nb.logger = l.(*logger)
	return nb
}

func droppedCount(t *testing.T, nb *NonBlockingWriter) int {
	t.Helper()

	var m dto.Metric
	if err := nb.metrics.DroppedCount.Write(&m); err != nil {
		t.Fatal(err)
	}
	return int(m.GetCounter().GetValue())
}

// stall writes total messages to nb so that the first one is held by the
// stalled sink, the next size ones fill the queue and the rest are dropped.
func stall(t *testing.T, nb *NonBlockingWriter, sink *stalledWriter, total int) {
	t.Helper()

	// Wait for the first message to be held by the
	// stalled sink, so that the queue is empty again.
	_, _ = fmt.Fprintf(nb, "message %d\n", 0)
	select {
	case <-sink.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first message did not reach the sink")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 1; i < total; i++ {
			_, _ = fmt.Fprintf(nb, "message %d\n", i)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer blocked on a stalled sink")
	}
}

func droppedNotice(n int) string {
	return fmt.Sprintf(`"level"="warning" "logger"="root" "msg"="log output is stalled, messages were dropped" "dropped"=%d`+"\n", n)
}

func TestNonBlockingWriter(t *testing.T) {
	t.Parallel()

	const (
		size        = 4
		total       = 100
		wantDropped = total - 1 - size
		kept        = "message 0\nmessage 1\nmessage 2\nmessage 3\nmessage 4\n"
	)

	t.Run("stalled sink does not block and drops are reported where they happened", func(t *testing.T) {
		t.Parallel()

		sink := newStalledWriter()
		nb := newNonBlockingWriter(sink, size)
		t.Cleanup(func() { sink.unblock(); _ = nb.Close() })

		stall(t, nb, sink, total)
		if got := droppedCount(t, nb); got != wantDropped {
			t.Fatalf("dropped count: got %d, want %d", got, wantDropped)
		}

		sink.unblock()
		deadline := time.Now().Add(5 * time.Second)
		for len(nb.queue) > 0 {
			if time.Now().After(deadline) {
				t.Fatal("queue was not drained")
			}
			time.Sleep(time.Millisecond)
		}
		_, _ = fmt.Fprintln(nb, "after recovery")
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		want := kept + droppedNotice(wantDropped) + "after recovery\n"
		if got := sink.buf.String(); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("drops before close are reported", func(t *testing.T) {
		t.Parallel()

		sink := newStalledWriter()
		nb := newNonBlockingWriter(sink, size)
		t.Cleanup(func() { sink.unblock(); _ = nb.Close() })

		stall(t, nb, sink, total)
		sink.unblock()
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		want := kept + droppedNotice(wantDropped)
		if got := sink.buf.String(); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("slow output is waited for", func(t *testing.T) {
		t.Parallel()

		sink := &slowWriter{delay: 5 * time.Millisecond}
		nb := newNonBlockingWriter(sink, 1)
		nb.stallTimeout = 5 * time.Second

		for i := range 20 {
			_, _ = fmt.Fprintf(nb, "%d\n", i)
		}
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		if got := droppedCount(t, nb); got != 0 {
			t.Fatalf("dropped count: got %d, want 0", got)
		}
		if got := bytes.Count(sink.buf.Bytes(), []byte("\n")); got != 20 {
			t.Fatalf("written messages: got %d, want 20", got)
		}
	})

	t.Run("healthy output under concurrent load drops nothing", func(t *testing.T) {
		t.Parallel()

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			_, _ = io.Copy(io.Discard, r)
		}()

		nb := newNonBlockingWriter(w, 64)
		nb.stallTimeout = 5 * time.Second
		l, _ := newLogger(WithVerbosity(VerbosityAll))
		l.(*logger).sink = nb

		const goroutines, lines = 4, 5000
		var wg sync.WaitGroup
		for range goroutines {
			wg.Go(func() {
				for i := range lines {
					l.(*logger).Debug("message", "n", i)
				}
			})
		}
		wg.Wait()
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
		<-drained
		_ = r.Close()

		if got := droppedCount(t, nb); got != 0 {
			t.Fatalf("dropped count: got %d, want 0", got)
		}
	})

	t.Run("logger on a stalled pipe does not block", func(t *testing.T) {
		t.Parallel()

		// Nothing reads r, so the pipe fills up and writes to w block.
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		nb := newNonBlockingWriter(w, 16)
		l, _ := newLogger(WithVerbosity(VerbosityAll))
		l.(*logger).sink = nb
		t.Cleanup(func() {
			_ = r.Close() // Unblocks the stalled write.
			_ = nb.Close()
			_ = w.Close()
		})

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 2000 {
				l.(*logger).Warning("could not send payment threshold announcement to peer",
					"peer_address", "0000000000000000000000000000000000000000000000000000000000000000")
			}
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("logger blocked on a stalled pipe")
		}

		if got := droppedCount(t, nb); got == 0 {
			t.Fatal("expected dropped messages")
		}
	})

	t.Run("failed writes are counted", func(t *testing.T) {
		t.Parallel()

		nb := newNonBlockingWriter(failingWriter{}, 16)
		for range 3 {
			_, _ = fmt.Fprintln(nb, "message")
		}
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		if got := droppedCount(t, nb); got != 3 {
			t.Fatalf("dropped count: got %d, want 3", got)
		}
	})

	t.Run("close flushes queued messages", func(t *testing.T) {
		t.Parallel()

		var sink bytes.Buffer
		nb := newNonBlockingWriter(&sink, 16)

		for i := range 10 {
			if _, err := fmt.Fprintf(nb, "%d\n", i); err != nil {
				t.Fatal(err)
			}
		}
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		want := "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n"
		if got := sink.String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("writes after close are discarded", func(t *testing.T) {
		t.Parallel()

		var sink bytes.Buffer
		nb := newNonBlockingWriter(&sink, 16)
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		if _, err := fmt.Fprintln(nb, "message"); err != nil {
			t.Fatal(err)
		}
		if n := len(nb.queue); n != 0 {
			t.Fatalf("queued messages: got %d, want 0", n)
		}
		if got := droppedCount(t, nb); got != 0 {
			t.Fatalf("dropped count: got %d, want 0", got)
		}
		if sink.Len() != 0 {
			t.Fatalf("unexpected output: %q", sink.String())
		}
	})

	t.Run("close returns when the sink is stalled", func(t *testing.T) {
		t.Parallel()

		sink := newStalledWriter()
		t.Cleanup(sink.unblock)

		nb := newNonBlockingWriter(sink, 16)
		nb.closeTimeout = 50 * time.Millisecond

		if _, err := nb.Write([]byte("message\n")); err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = nb.Close()
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("close blocked on a stalled sink")
		}
	})
}

// TestNonBlockingWriterDropDoesNotAllocate is not parallel,
// because testing.AllocsPerRun counts allocations process wide.
func TestNonBlockingWriterDropDoesNotAllocate(t *testing.T) {
	sink := newStalledWriter()
	nb := newNonBlockingWriter(sink, 1)
	t.Cleanup(func() { sink.unblock(); _ = nb.Close() })

	stall(t, nb, sink, 2)

	p := []byte("message\n")
	if allocs := testing.AllocsPerRun(100, func() { _, _ = nb.Write(p) }); allocs != 0 {
		t.Fatalf("allocations per dropped message: got %v, want 0", allocs)
	}
}
