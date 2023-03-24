package log

import (
	"testing"
)

// NewTestLogger returns logger used for testing.
// This logger uses t.Log as sink for log outputs.
func NewTestLogger(t *testing.T, opts ...Option) Logger {
	opts = append(opts, WithSink(&testWriter{t: t}))

	return NewLogger(t.Name(), opts...)
}

type testWriter struct {
	t *testing.T
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	tw.t.Log(string(p))
	return len(p), nil
}
