// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/compute"
)

// loadModule reads a WASM fixture. See testdata/README.md for their sources.
func loadModule(t *testing.T, name string) []byte {
	t.Helper()

	module, err := os.ReadFile(filepath.Join("testdata", name+".wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func newService(t *testing.T, o compute.Options) *compute.Service {
	t.Helper()

	if o.Watchdog == 0 {
		o.Watchdog = 10 * time.Second
	}
	s, err := compute.New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close compute service: %v", err)
		}
	})
	return s
}

func TestExecute(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		module string
		input  []byte
		limits compute.Limits
		want   compute.Status
		output string
	}{
		{
			name:   "writes to stdout",
			module: "writer",
			want:   compute.StatusOK,
			output: "hello swarm",
		},
		{
			name:   "input is echoed back",
			module: "echo",
			input:  []byte("swarm input"),
			want:   compute.StatusOK,
			output: "swarm input",
		},
		{
			name:   "explicit entrypoint",
			module: "entrypoint",
			limits: compute.Limits{Entrypoint: "run"},
			want:   compute.StatusOK,
			output: "entrypoint output",
		},
		{
			name:   "entrypoint not exported",
			module: "writer",
			limits: compute.Limits{Entrypoint: "missing"},
			want:   compute.StatusInvalidModule,
		},
		{
			// Without an explicit entrypoint the module has no WASI command
			// entry to run.
			name:   "no start function",
			module: "entrypoint",
			want:   compute.StatusInvalidModule,
		},
		{
			name:   "unreachable traps",
			module: "trap",
			want:   compute.StatusTrap,
		},
		{
			name:   "non-zero exit traps",
			module: "exit1",
			want:   compute.StatusTrap,
		},
		{
			name:   "unsupported import",
			module: "badimport",
			want:   compute.StatusInvalidModule,
		},
		{
			name:   "memory over the limit",
			module: "bigmem",
			limits: compute.Limits{Memory: 64 * 1024},
			want:   compute.StatusInvalidModule,
		},
		{
			name:   "memory within the limit",
			module: "bigmem",
			limits: compute.Limits{Memory: 16 * 1024 * 1024},
			want:   compute.StatusOK,
			output: "big",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newService(t, compute.Options{Workers: 1})

			res, err := s.Execute(context.Background(), compute.Request{
				Module: loadModule(t, tc.module),
				Input:  tc.input,
				Limits: tc.limits,
			})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if res.Status != tc.want {
				t.Errorf("got status %v, want %v (trap message: %q)", res.Status, tc.want, res.TrapMessage)
			}
			if string(res.Output) != tc.output {
				t.Errorf("got output %q, want %q", res.Output, tc.output)
			}
			if tc.want != compute.StatusOK && res.TrapMessage == "" {
				t.Error("want a trap message explaining the verdict")
			}
		})
	}
}

func TestExecuteInvalidModule(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{Module: []byte("this is not a wasm module")})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != compute.StatusInvalidModule {
		t.Errorf("got status %v, want %v", res.Status, compute.StatusInvalidModule)
	}
}

// TestExecuteWatchdog checks that a module which never terminates is killed and
// reported as an infrastructure failure rather than as a program verdict: the
// kill depends on this node's wall clock, so it is not reproducible elsewhere.
func TestExecuteWatchdog(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1, Watchdog: 100 * time.Millisecond})

	res, err := s.Execute(context.Background(), compute.Request{Module: loadModule(t, "infloop")})
	if err == nil {
		t.Fatal("want an error for a watchdog kill")
	}
	if res.Status != compute.StatusHostError {
		t.Errorf("got status %v, want %v", res.Status, compute.StatusHostError)
	}
}

func TestExecuteContextCanceled(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	res, err := s.Execute(ctx, compute.Request{Module: loadModule(t, "infloop")})
	if err == nil {
		t.Fatal("want an error for a canceled execution")
	}
	if res.Status != compute.StatusHostError {
		t.Errorf("got status %v, want %v", res.Status, compute.StatusHostError)
	}
}

func TestExecuteBusy(t *testing.T) {
	t.Parallel()

	// With a single worker and a module that runs until the watchdog fires,
	// whichever execution acquires the worker holds it for the duration, so the
	// other must be rejected immediately instead of queueing behind it.
	s := newService(t, compute.Options{Workers: 1, Watchdog: time.Second})
	module := loadModule(t, "infloop")

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := s.Execute(context.Background(), compute.Request{Module: module})
			errs <- err
		}()
	}

	var busy int
	for i := 0; i < 2; i++ {
		if errors.Is(<-errs, compute.ErrBusy) {
			busy++
		}
	}
	if busy != 1 {
		t.Errorf("got %d executions rejected as busy, want exactly 1", busy)
	}
}

// TestExecuteNoStateLeak checks that consecutive executions of the same module
// do not observe each other's state.
func TestExecuteNoStateLeak(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 2})
	module := loadModule(t, "echo")

	for _, input := range []string{"first", "second", "third"} {
		res, err := s.Execute(context.Background(), compute.Request{Module: module, Input: []byte(input)})
		if err != nil {
			t.Fatalf("execute %q: %v", input, err)
		}
		if res.Status != compute.StatusOK {
			t.Fatalf("got status %v, want %v", res.Status, compute.StatusOK)
		}
		if string(res.Output) != input {
			t.Errorf("got output %q, want %q", res.Output, input)
		}
	}
}

func TestExecuteRequestMethod(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		method string
		want   string
	}{
		{name: "post", method: "POST", want: "REQUEST_METHOD=POST"},
		{name: "get", method: "GET", want: "REQUEST_METHOD=GET"},
		{name: "custom", method: "PROPFIND", want: "REQUEST_METHOD=PROPFIND"},
		{
			// No method means no environment at all, so the guest must not see
			// a stray entry from the host.
			name:   "unset",
			method: "",
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newService(t, compute.Options{Workers: 1})

			res, err := s.Execute(context.Background(), compute.Request{
				Module: loadModule(t, "method"),
				Method: tc.method,
			})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if res.Status != compute.StatusOK {
				t.Fatalf("got status %v, want %v (trap message: %q)", res.Status, compute.StatusOK, res.TrapMessage)
			}
			if string(res.Output) != tc.want {
				t.Errorf("got output %q, want %q", res.Output, tc.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		status compute.Status
		want   string
	}{
		{compute.StatusOK, "ok"},
		{compute.StatusOutOfFuel, "out-of-fuel"},
		{compute.StatusTrap, "trap"},
		{compute.StatusInvalidModule, "invalid-module"},
		{compute.StatusHostError, "host-error"},
		{compute.Status(0), "unknown"},
	} {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
