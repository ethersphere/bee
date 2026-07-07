// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/safe"
)

func TestGo(t *testing.T) {
	logger := &mockLogger{logged: make(chan struct{})}

	safe.Go(logger, "test-panic-goroutine", func() {
		panic("intentional panic async")
	})

	<-logger.logged

	msg, keyvals, err := logger.getLogged()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if msg != "goroutine panic recovered" {
		t.Errorf("expected message 'goroutine panic recovered', got %q", msg)
	}

	keyvalsMap := make(map[string]any)
	for i := 0; i < len(keyvals); i += 2 {
		keyvalsMap[keyvals[i].(string)] = keyvals[i+1]
	}

	if keyvalsMap["name"] != "test-panic-goroutine" {
		t.Errorf("expected name 'test-panic-goroutine', got %v", keyvalsMap["name"])
	}
	if keyvalsMap["panic"] != "intentional panic async" {
		t.Errorf("expected panic 'intentional panic async', got %v", keyvalsMap["panic"])
	}
	stack, ok := keyvalsMap["stack"].(string)
	if !ok || !strings.Contains(stack, "safe_test.go") {
		t.Errorf("expected stack trace containing safe_test.go, got %q", stack)
	}
}

func TestRun(t *testing.T) {
	logger := &mockLogger{logged: make(chan struct{})}

	safe.Run(logger, "test-panic-sync", func() {
		panic("intentional panic sync")
	})

	msg, keyvals, err := logger.getLogged()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if msg != "panic recovered" {
		t.Errorf("expected message 'panic recovered', got %q", msg)
	}

	keyvalsMap := make(map[string]any)
	for i := 0; i < len(keyvals); i += 2 {
		keyvalsMap[keyvals[i].(string)] = keyvals[i+1]
	}

	if keyvalsMap["name"] != "test-panic-sync" {
		t.Errorf("expected name 'test-panic-sync', got %v", keyvalsMap["name"])
	}
	if keyvalsMap["panic"] != "intentional panic sync" {
		t.Errorf("expected panic 'intentional panic sync', got %v", keyvalsMap["panic"])
	}
	stack, ok := keyvalsMap["stack"].(string)
	if !ok || !strings.Contains(stack, "safe_test.go") {
		t.Errorf("expected stack trace containing safe_test.go, got %q", stack)
	}
}

func TestRunFunc(t *testing.T) {
	t.Run("no panic", func(t *testing.T) {
		logger := &mockLogger{logged: make(chan struct{})}

		wrapped := safe.RunFunc(logger, "test-run-func-ok", func() error {
			return nil
		})

		err := wrapped()
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("with panic", func(t *testing.T) {
		logger := &mockLogger{logged: make(chan struct{})}

		wrapped := safe.RunFunc(logger, "test-run-func-panic", func() error {
			panic("intentional panic runfunc")
		})

		err := wrapped()
		if err == nil {
			t.Error("expected non-nil error from panic recovery, got nil")
		} else if !strings.Contains(err.Error(), "intentional panic runfunc") {
			t.Errorf("expected error message to contain panic value, got %q", err.Error())
		}

		<-logger.logged

		msg, keyvals, _ := logger.getLogged()
		if msg != "errgroup goroutine panic recovered" {
			t.Errorf("expected message 'errgroup goroutine panic recovered', got %q", msg)
		}

		keyvalsMap := make(map[string]any)
		for i := 0; i < len(keyvals); i += 2 {
			keyvalsMap[keyvals[i].(string)] = keyvals[i+1]
		}

		if keyvalsMap["name"] != "test-run-func-panic" {
			t.Errorf("expected name 'test-run-func-panic', got %v", keyvalsMap["name"])
		}
		if keyvalsMap["panic"] != "intentional panic runfunc" {
			t.Errorf("expected panic 'intentional panic runfunc', got %v", keyvalsMap["panic"])
		}
		stack, ok := keyvalsMap["stack"].(string)
		if !ok || !strings.Contains(stack, "safe_test.go") {
			t.Errorf("expected stack trace containing safe_test.go, got %q", stack)
		}
	})

	t.Run("with panic error wrapping", func(t *testing.T) {
		logger := &mockLogger{logged: make(chan struct{})}

		type customErr struct {
			error
		}
		var sentinelErr = customErr{error: errors.New("sentinel panic")}

		wrapped := safe.RunFunc(logger, "test-run-func-panic-err", func() error {
			panic(sentinelErr)
		})

		err := wrapped()
		if err == nil {
			t.Error("expected non-nil error from panic recovery, got nil")
		} else if !errors.Is(err, sentinelErr) {
			t.Errorf("expected wrapped error to be errors.Is sentinelErr, got %v", err)
		}
	})

	t.Run("nil logger", func(t *testing.T) {
		wrapped := safe.RunFunc(nil, "test-run-func-nil-logger", func() error {
			panic("panic without logger")
		})

		err := wrapped()
		if err == nil {
			t.Error("expected non-nil error from panic recovery, got nil")
		} else if !strings.Contains(err.Error(), "panic without logger") {
			t.Errorf("expected error message to contain panic value, got %q", err.Error())
		}
	})
}

type mockLogger struct {
	log.Logger
	loggedErr     error
	loggedMsg     string
	loggedKeyvals []any
	mtx           sync.Mutex
	logged        chan struct{}
}

func (m *mockLogger) Error(err error, msg string, keyvals ...any) {
	m.mtx.Lock()
	m.loggedErr = err
	m.loggedMsg = msg
	m.loggedKeyvals = keyvals
	m.mtx.Unlock()
	close(m.logged)
}

func (m *mockLogger) getLogged() (string, []any, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.loggedMsg, m.loggedKeyvals, m.loggedErr
}
