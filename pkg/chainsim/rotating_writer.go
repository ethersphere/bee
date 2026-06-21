// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingJSONLWriter implements JSONLWriter by writing events to disk files
// that rotate every interval of wall-clock time since the writer was created.
type RotatingJSONLWriter struct {
	mu       sync.Mutex
	dir      string
	interval time.Duration
	start    time.Time
	curIdx   int
	file     *os.File
	buf      *bufio.Writer
}

// NewRotatingJSONLWriter creates a writer that rotates log files in dir every interval.
func NewRotatingJSONLWriter(dir string, interval time.Duration) (*RotatingJSONLWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &RotatingJSONLWriter{
		dir:      dir,
		interval: interval,
		start:    time.Now(),
		curIdx:   -1,
	}
	if err := w.rotateTo(0); err != nil {
		return nil, err
	}
	return w, nil
}

var _ JSONLWriter = (*RotatingJSONLWriter)(nil)

// CurrentFileName returns the base name of the file currently being written.
func (w *RotatingJSONLWriter) CurrentFileName() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return ""
	}
	return filepath.Base(w.file.Name())
}

// WriteEvent writes a single event, rotating the underlying file if needed.
func (w *RotatingJSONLWriter) WriteEvent(e JSONLEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()

	idx := int(time.Since(w.start) / w.interval)
	if idx != w.curIdx {
		_ = w.rotateToLocked(idx)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = w.buf.Write(line)
	_ = w.buf.WriteByte('\n')
	_ = w.buf.Flush()
}

func (w *RotatingJSONLWriter) rotateTo(idx int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotateToLocked(idx)
}

func (w *RotatingJSONLWriter) rotateToLocked(idx int) error {
	if w.buf != nil {
		_ = w.buf.Flush()
	}
	if w.file != nil {
		_ = w.file.Close()
	}

	name := w.fileName(idx)
	f, err := os.Create(filepath.Join(w.dir, name))
	if err != nil {
		return err
	}
	w.file = f
	w.buf = bufio.NewWriter(f)
	w.curIdx = idx
	return nil
}

func (w *RotatingJSONLWriter) fileName(idx int) string {
	if w.interval >= time.Minute {
		startMin := idx * int(w.interval/time.Minute)
		endMin := (idx + 1) * int(w.interval/time.Minute)
		return fmt.Sprintf("events_%03dm-%03dm.jsonl", startMin, endMin)
	}
	startSec := idx * int(w.interval/time.Second)
	endSec := (idx + 1) * int(w.interval/time.Second)
	return fmt.Sprintf("events_%04ds-%04ds.jsonl", startSec, endSec)
}

// Close flushes and closes the current file.
func (w *RotatingJSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf != nil {
		_ = w.buf.Flush()
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
