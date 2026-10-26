// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"fmt"
	"io"
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

// newNonBlockingWriter returns a NonBlockingWriter whose dropped messages
// notice is logged without pinning the defaults, see newLogger.
func newNonBlockingWriter(w io.Writer, size int) *NonBlockingWriter {
	nb := NewNonBlockingWriter(w, size)
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

func TestNonBlockingWriter(t *testing.T) {
	t.Parallel()

	t.Run("stalled sink does not block the logger", func(t *testing.T) {
		t.Parallel()

		const (
			size  = 4
			total = 100
		)

		sink := newStalledWriter()
		nb := newNonBlockingWriter(sink, size)
		t.Cleanup(func() { sink.unblock(); _ = nb.Close() })

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
			t.Fatal("logger blocked on a stalled sink")
		}

		wantDropped := total - 1 - size
		if got := droppedCount(t, nb); got != wantDropped {
			t.Fatalf("dropped count: got %d, want %d", got, wantDropped)
		}

		sink.unblock()
		if err := nb.Close(); err != nil {
			t.Fatal(err)
		}

		want := fmt.Sprintf("message 0\n"+
			`"level"="warning" "logger"="root" "msg"="log output is not keeping up, messages were dropped" "dropped"=%d`+"\n"+
			"message 1\nmessage 2\nmessage 3\nmessage 4\n", wantDropped)
		if got := sink.buf.String(); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
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
