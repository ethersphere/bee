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
// goroutine through a bounded queue, so that a stalled underlying writer
// (an unread pipe, a full disk, a paused log collector) does not block the
// caller. While the underlying writer makes progress, a full queue makes the
// caller wait for space, as a synchronous writer would. Once a single write
// has been stuck for longer than the stall timeout, messages are dropped and
// counted instead, and the count is logged once the writer recovers.
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

	// stallTimeout is how long a single write to the underlying
	// writer may take before the writer is considered stalled.
	stallTimeout time.Duration

	// epoch is the monotonic reference for writeStart.
	epoch time.Time
	// writeStart is the time since epoch, plus one, when the current write
	// to the underlying writer started, or zero if no write is in progress.
	writeStart atomic.Int64

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
		stallTimeout: time.Second,
		epoch:        time.Now(),
	}
	go nb.run()
	return nb
}

// Write implements the io.Writer interface. It never returns an error and
// blocks for at most the stall timeout; if the underlying writer is stalled
// and the queue is full, the message is dropped.
func (nb *NonBlockingWriter) Write(p []byte) (int, error) {
	select {
	case <-nb.quit:
		return len(p), nil
	default:
	}

	// Check before copying, so that dropping a message does not allocate.
	if len(nb.queue) == cap(nb.queue) && nb.stalledFor() >= nb.stallTimeout {
		nb.drop(1)
		return len(p), nil
	}

	dropped := nb.dropped.Swap(0)
	msg := queuedMessage{p: bytes.Clone(p), dropped: dropped}
	select {
	case nb.queue <- msg:
		return len(p), nil
	default:
	}

	// The queue is full: wait for space while the underlying writer makes
	// progress, and drop the message once its current write is stalled.
	timer := time.NewTimer(nb.stallTimeout)
	defer timer.Stop()
	for {
		wait := nb.stallTimeout - nb.stalledFor()
		if wait <= 0 {
			nb.dropped.Add(dropped)
			nb.drop(1)
			return len(p), nil
		}
		timer.Reset(wait)
		select {
		case nb.queue <- msg:
			return len(p), nil
		case <-timer.C:
		case <-nb.quit:
			return len(p), nil
		}
	}
}

// stalledFor returns for how long the current write
// to the underlying writer has been in progress.
func (nb *NonBlockingWriter) stalledFor() time.Duration {
	start := nb.writeStart.Load()
	if start == 0 {
		return 0
	}
	return time.Since(nb.epoch) - time.Duration(start-1)
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
	nb.writeStart.Store(int64(time.Since(nb.epoch)) + 1)
	nb.reportDropped(msg.dropped)
	_, err := nb.w.Write(msg.p)
	nb.writeStart.Store(0)
	if err != nil {
		nb.metrics.DroppedCount.Inc()
	}
}

func (nb *NonBlockingWriter) reportDropped(n uint64) {
	if n == 0 {
		return
	}
	if nb.logger == nil {
		nb.logger = NewLogger("log", WithSink(nb.w), WithVerbosity(VerbosityWarning))
	}
	nb.logger.Warning("log output is stalled, messages were dropped", "dropped", n)
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
			Help:      "Number of log messages that were not written to the log output.",
		}),
	}
}
