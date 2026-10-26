// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"time"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	_ io.WriteCloser = (*NonBlockingWriter)(nil)
	_ m.Collector    = (*NonBlockingWriter)(nil)
)

// NonBlockingWriter is an io.Writer that hands messages over to a background
// goroutine through a bounded queue, so that a slow or stalled underlying
// writer (an unread pipe, a full disk, a paused log collector) never blocks
// the caller. When the queue is full, messages are dropped and counted, and
// the count is logged once the underlying writer accepts writes again.
type NonBlockingWriter struct {
	w       io.Writer
	queue   chan queuedMessage
	dropped atomic.Uint64 // dropped messages not yet reported
	quit    chan struct{}
	done    chan struct{}
	close   sync.Once
	metrics nonBlockingWriterMetrics

	// closeTimeout bounds how long Close waits for the
	// queued messages to be written to the underlying writer.
	closeTimeout time.Duration

	// logger reports dropped messages directly to w. It is created lazily
	// by the writing goroutine so that it inherits the default options.
	logger Logger
}

// queuedMessage is a message waiting to be written, together with
// the number of messages dropped right before it was queued.
type queuedMessage struct {
	p       []byte
	dropped uint64
}

// NewNonBlockingWriter returns a NonBlockingWriter that writes to w
// and queues at most size messages. Close must be called to release it.
func NewNonBlockingWriter(w io.Writer, size int) *NonBlockingWriter {
	nb := &NonBlockingWriter{
		w:       w,
		queue:   make(chan queuedMessage, size),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
		metrics: newNonBlockingWriterMetrics(),

		closeTimeout: 5 * time.Second,
	}
	go nb.run()
	return nb
}

// Write implements the io.Writer interface. It never blocks and never
// returns an error; if the queue is full, the message is dropped.
func (nb *NonBlockingWriter) Write(p []byte) (int, error) {
	select {
	case <-nb.quit:
		return len(p), nil
	default:
	}

	// Check before copying, so that dropping a message does not allocate.
	if len(nb.queue) == cap(nb.queue) {
		nb.drop(1)
		return len(p), nil
	}

	dropped := nb.dropped.Swap(0)
	select {
	case nb.queue <- queuedMessage{p: bytes.Clone(p), dropped: dropped}:
	default:
		nb.dropped.Add(dropped)
		nb.drop(1)
	}
	return len(p), nil
}

func (nb *NonBlockingWriter) drop(n uint64) {
	nb.dropped.Add(n)
	nb.metrics.DroppedCount.Add(float64(n))
}

// Close stops the writing goroutine after the queued messages are written,
// waiting for at most closeTimeout. Messages written after Close are
// discarded.
func (nb *NonBlockingWriter) Close() error {
	nb.close.Do(func() { close(nb.quit) })
	select {
	case <-nb.done:
	case <-time.After(nb.closeTimeout):
	}
	return nil
}

// Metrics implements metrics.Collector interface.
func (nb *NonBlockingWriter) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(nb.metrics)
}

func (nb *NonBlockingWriter) run() {
	defer close(nb.done)
	for {
		select {
		case msg := <-nb.queue:
			nb.write(msg)
		case <-nb.quit:
			for {
				select {
				case msg := <-nb.queue:
					nb.write(msg)
				default:
					nb.reportDropped(nb.dropped.Swap(0))
					return
				}
			}
		}
	}
}

func (nb *NonBlockingWriter) write(msg queuedMessage) {
	nb.reportDropped(msg.dropped)
	_, _ = nb.w.Write(msg.p)
}

func (nb *NonBlockingWriter) reportDropped(n uint64) {
	if n == 0 {
		return
	}
	if nb.logger == nil {
		nb.logger = NewLogger("log", WithSink(nb.w), WithVerbosity(VerbosityWarning))
	}
	nb.logger.Warning("log output is not keeping up, messages were dropped", "dropped", n)
}

type nonBlockingWriterMetrics struct {
	DroppedCount prometheus.Counter
}

func newNonBlockingWriterMetrics() nonBlockingWriterMetrics {
	return nonBlockingWriterMetrics{
		DroppedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "log",
			Name:      "dropped_count",
			Help:      "Number of log messages dropped because the log output was not keeping up.",
		}),
	}
}
