// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
)

// JSONLEvent is a single structured log event written to a JSONL file.
type JSONLEvent struct {
	Timestamp string         `json:"ts"`
	Source    string         `json:"source"`
	Level     string         `json:"level"`
	Message   string         `json:"msg"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// JSONLWriter receives structured log events.
type JSONLWriter interface {
	WriteEvent(event JSONLEvent)
}

// JSONLLogger implements log.Logger by serializing every log call as a JSON line.
type JSONLLogger struct {
	source string
	writer JSONLWriter
	extra  map[string]any
	name   string
}

// NewJSONLLogger creates a logger that writes structured JSONL events to w.
func NewJSONLLogger(source string, w JSONLWriter) *JSONLLogger {
	return &JSONLLogger{source: source, writer: w}
}

var _ log.Logger = (*JSONLLogger)(nil)

func (l *JSONLLogger) emit(level, msg string, kvs []any) {
	fields := make(map[string]any, len(l.extra)+len(kvs)/2)
	for k, v := range l.extra {
		fields[k] = v
	}
	if l.name != "" {
		fields["logger_name"] = l.name
	}
	for i := 0; i+1 < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			key = fmt.Sprintf("key_%d", i)
		}
		fields[key] = kvs[i+1]
	}
	l.writer.WriteEvent(JSONLEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    l.source,
		Level:     level,
		Message:   msg,
		Fields:    fields,
	})
}

func (l *JSONLLogger) Debug(msg string, kvs ...any)   { l.emit("debug", msg, kvs) }
func (l *JSONLLogger) Info(msg string, kvs ...any)    { l.emit("info", msg, kvs) }
func (l *JSONLLogger) Warning(msg string, kvs ...any) { l.emit("warning", msg, kvs) }
func (l *JSONLLogger) Error(err error, msg string, kvs ...any) {
	if err != nil {
		kvs = append([]any{"error", err.Error()}, kvs...)
	}
	l.emit("error", msg, kvs)
}
func (l *JSONLLogger) Verbosity() log.Level { return log.VerbosityAll }

func (l *JSONLLogger) V(_ uint) log.Builder { return l }
func (l *JSONLLogger) WithName(name string) log.Builder {
	c := l.clone()
	if c.name != "" {
		c.name += "/" + name
	} else {
		c.name = name
	}
	return c
}
func (l *JSONLLogger) WithValues(kvs ...any) log.Builder {
	c := l.clone()
	for i := 0; i+1 < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			key = fmt.Sprintf("key_%d", i)
		}
		c.extra[key] = kvs[i+1]
	}
	return c
}
func (l *JSONLLogger) Build() log.Logger    { return l }
func (l *JSONLLogger) Register() log.Logger { return l }

func (l *JSONLLogger) clone() *JSONLLogger {
	c := &JSONLLogger{source: l.source, writer: l.writer, name: l.name}
	c.extra = make(map[string]any, len(l.extra))
	for k, v := range l.extra {
		c.extra[k] = v
	}
	return c
}

// JSONLFile is a thread-safe JSONLWriter that appends to an in-memory buffer.
type JSONLFile struct {
	mu     sync.Mutex
	events []JSONLEvent
}

func (f *JSONLFile) WriteEvent(e JSONLEvent) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

// Events returns a copy of all collected events.
func (f *JSONLFile) Events() []JSONLEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]JSONLEvent, len(f.events))
	copy(out, f.events)
	return out
}

// MarshalJSONL returns all events as newline-delimited JSON.
func (f *JSONLFile) MarshalJSONL() ([]byte, error) {
	events := f.Events()
	var buf []byte
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	return buf, nil
}
