// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute_test

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/compute"
)

// headerValues collects the values a guest set for one name, in order.
func headerValues(meta compute.ResponseMeta, name string) []string {
	var out []string
	for _, h := range meta.Headers {
		if strings.EqualFold(h.Name, name) {
			out = append(out, h.Value)
		}
	}
	return out
}

func TestResponseMetadata(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{
		Module: loadModule(t, "respok"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
	}
	if string(res.Output) != "hi" {
		t.Errorf("output: got %q, want %q", res.Output, "hi")
	}
	if res.Response.Status != 201 {
		t.Errorf("status: got %d, want 201", res.Response.Status)
	}
	if got := headerValues(res.Response, "Content-Type"); len(got) != 1 || got[0] != "text/css" {
		t.Errorf("content-type: got %v, want [text/css]", got)
	}
	if got := headerValues(res.Response, "Cache-Control"); len(got) != 1 || got[0] != "max-age=60" {
		t.Errorf("cache-control: got %v, want [max-age=60]", got)
	}
	// Repeats are kept rather than collapsed: Link and Vary legitimately repeat.
	if got := headerValues(res.Response, "Link"); len(got) != 1 || got[0] != "</a>; rel=next" {
		t.Errorf("link: got %v", got)
	}
	if res.Response.Empty() {
		t.Error("Empty() reported true for a guest that set metadata")
	}
}

// A module that never calls the response functions must look exactly as it did
// before they existed, which is what lets the API layer keep its old behaviour.
func TestResponseMetadataAbsent(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{
		Module: loadModule(t, "writer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Response.Empty() {
		t.Errorf("Empty(): got false, want true (%+v)", res.Response)
	}
}

func TestResponseRefusals(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{
		Module: loadModule(t, "respbad"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v (nothing may trap)", res.Status, compute.StatusOK)
	}
	if len(res.Output) != 24 {
		t.Fatalf("output: got %d bytes, want 24", len(res.Output))
	}

	for i, tc := range []struct {
		name string
		want uint32
	}{
		{"CR/LF in a header name", errnoInvalid},
		{"forging Swarm-Wasm-Status", errnoDenied},
		{"widening CORS", errnoDenied},
		{"setting a cookie", errnoDenied},
		{"a 99 status code", errnoInvalid},
		{"an absurd value length", errnoInvalid},
	} {
		if got := binary.LittleEndian.Uint32(res.Output[i*4:]); got != tc.want {
			t.Errorf("%s: got errno %d, want %d", tc.name, got, tc.want)
		}
	}

	// Every call was refused, so nothing may have been recorded.
	if !res.Response.Empty() {
		t.Errorf("refused calls left metadata behind: %+v", res.Response)
	}
}

func TestResponseHeaderCaps(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		limits  compute.Limits
		wantMax int
	}{
		{
			// 15 bytes a header ("X-Pad" + 10), so the count cap binds first.
			name:    "count cap",
			limits:  compute.Limits{MaxResponseHeaders: 4, MaxResponseHeaderBytes: 8 << 10},
			wantMax: 4,
		},
		{
			name:    "byte cap",
			limits:  compute.Limits{MaxResponseHeaders: 1000, MaxResponseHeaderBytes: 45},
			wantMax: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newService(t, compute.Options{Workers: 1})

			res, err := s.Execute(context.Background(), compute.Request{
				Module: loadModule(t, "respflood"),
				Limits: tc.limits,
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != compute.StatusOK {
				t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
			}

			accepted := int(binary.LittleEndian.Uint32(res.Output[0:]))
			code := binary.LittleEndian.Uint32(res.Output[4:])
			if accepted != tc.wantMax {
				t.Errorf("accepted: got %d, want %d", accepted, tc.wantMax)
			}
			if code != errnoBudgetExhausted {
				t.Errorf("stopping errno: got %d, want %d", code, errnoBudgetExhausted)
			}
			if len(res.Response.Headers) != tc.wantMax {
				t.Errorf("recorded headers: got %d, want %d", len(res.Response.Headers), tc.wantMax)
			}
		})
	}
}

// A trap discards response metadata the way it discards uploads, while partial
// output survives as evidence. The asymmetry is deliberate.
func TestResponseMetadataDroppedOnTrap(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{
		Module: loadModule(t, "resptrap"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != compute.StatusTrap {
		t.Fatalf("status: got %v, want %v", res.Status, compute.StatusTrap)
	}
	if !res.Response.Empty() {
		t.Errorf("a trapped module kept its response metadata: %+v", res.Response)
	}
	if string(res.Output) != "part" {
		t.Errorf("output: got %q, want %q (partial output is evidence)", res.Output, "part")
	}
}

// Shaping the response causes no node work, so it must not require a Host. The
// data functions must still be rejected in that configuration.
func TestResponseWithoutNodeAccess(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 1})

	res, err := s.Execute(context.Background(), compute.Request{
		Module: loadModule(t, "respok"),
		// No Host.
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
	}
	if res.Response.Status != 201 {
		t.Errorf("status: got %d, want 201", res.Response.Status)
	}
}

// A module reached through swarm_execute is a library call, not an HTTP request.
// Letting it set headers would let a module fetched from Swarm rewrite its
// caller's response, so it is refused outright.
func TestResponseRefusedWhenNested(t *testing.T) {
	t.Parallel()

	t.Run("outermost is allowed", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		res := runHost(t, host, "respcode", nil, compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
		}
		if got := binary.LittleEndian.Uint32(res.Output); got != errnoOK {
			t.Errorf("errno: got %d, want %d", got, errnoOK)
		}
		if got := headerValues(res.Response, "X-Depth"); len(got) != 1 {
			t.Errorf("header not recorded: %v", got)
		}
	})

	t.Run("nested is denied", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		addr := host.addData(2, loadModule(t, "respcode"))

		res := runHost(t, host, "hostnested", addr.Bytes(), compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
		}

		fields, data := splitOutput(t, res.Output, 2)
		if fields[0] != errnoOK {
			t.Fatalf("swarm_execute errno: got %d, want %d", fields[0], errnoOK)
		}
		if got := binary.LittleEndian.Uint32(data); got != errnoDenied {
			t.Errorf("nested errno: got %d, want %d", got, errnoDenied)
		}
		// The child's attempt must leave nothing on the parent's response.
		if !res.Response.Empty() {
			t.Errorf("a nested module set metadata on its caller: %+v", res.Response)
		}
	})
}

// splitEnv splits the environment block a guest dumped into its entries.
func splitEnv(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	return strings.Split(string(out), "\x00")
}

func TestExecuteEnv(t *testing.T) {
	t.Parallel()

	t.Run("entries reach the guest in the order given", func(t *testing.T) {
		t.Parallel()

		s := newService(t, compute.Options{Workers: 1})

		res, err := s.Execute(context.Background(), compute.Request{
			Module: loadModule(t, "method"),
			Method: "GET",
			Env: []compute.EnvVar{
				{Name: "SCRIPT_NAME", Value: "/@/abc"},
				{Name: "PATH_INFO", Value: "/style.css"},
				{Name: "QUERY_STRING", Value: "a=1&b=2"},
				{Name: "HTTP_ACCEPT", Value: "text/css"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		want := []string{
			// Method still comes first: it owns REQUEST_METHOD.
			"REQUEST_METHOD=GET",
			"SCRIPT_NAME=/@/abc",
			"PATH_INFO=/style.css",
			"QUERY_STRING=a=1&b=2",
			"HTTP_ACCEPT=text/css",
		}
		got := splitEnv(res.Output)
		if len(got) != len(want) {
			t.Fatalf("env: got %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("env[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("malformed entries are refused", func(t *testing.T) {
		t.Parallel()

		s := newService(t, compute.Options{Workers: 1})

		res, err := s.Execute(context.Background(), compute.Request{
			Module: loadModule(t, "method"),
			Method: "GET",
			Env: []compute.EnvVar{
				// A duplicate REQUEST_METHOD would give the guest two entries
				// for one name.
				{Name: "REQUEST_METHOD", Value: "PUT"},
				// "=" and NUL would corrupt the flat environ block.
				{Name: "BAD=NAME", Value: "x"},
				{Name: "BAD\x00NAME", Value: "x"},
				{Name: "OK_NAME", Value: "kept"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		want := []string{"REQUEST_METHOD=GET", "OK_NAME=kept"}
		got := splitEnv(res.Output)
		if len(got) != len(want) {
			t.Fatalf("env: got %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("env[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})
}
