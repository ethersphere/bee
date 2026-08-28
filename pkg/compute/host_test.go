// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Guest-visible result codes, mirrored from the ABI so a test asserts the
// number a module actually observes.
const (
	errnoOK              = 0
	errnoNotFound        = 1
	errnoDenied          = 2
	errnoBudgetExhausted = 3
	errnoBufferTooSmall  = 4
	errnoInvalid         = 5
	errnoExecFailed      = 6
)

// mockHost serves canned data and records what the guest asked for.
type mockHost struct {
	data   map[string][]byte
	chunks map[string][]byte

	// err, when set, is returned by every call, standing in for a node-local
	// failure that must never become a program verdict.
	err error
	// denyPut makes the puts report a refused batch.
	denyPut bool

	bytesGets int
	puts      [][]byte
}

func newMockHost() *mockHost {
	return &mockHost{data: map[string][]byte{}, chunks: map[string][]byte{}}
}

// addData stores payload under a synthetic address derived from seed.
func (m *mockHost) addData(seed byte, payload []byte) swarm.Address {
	addr := addressOf(seed)
	m.data[addr.String()] = payload
	return addr
}

func (m *mockHost) BytesGet(_ context.Context, addr swarm.Address) ([]byte, error) {
	m.bytesGets++
	if m.err != nil {
		return nil, m.err
	}
	payload, ok := m.data[addr.String()]
	if !ok {
		return nil, compute.ErrNotFound
	}
	return payload, nil
}

func (m *mockHost) BytesPut(_ context.Context, batchID, data []byte) (swarm.Address, error) {
	if m.err != nil {
		return swarm.ZeroAddress, m.err
	}
	if m.denyPut {
		return swarm.ZeroAddress, compute.ErrDenied
	}
	m.puts = append(m.puts, data)
	addr := addressOf(byte(len(m.data) + 1))
	m.data[addr.String()] = data
	return addr, nil
}

func (m *mockHost) ChunkGet(_ context.Context, addr swarm.Address) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	chunk, ok := m.chunks[addr.String()]
	if !ok {
		return nil, compute.ErrNotFound
	}
	return chunk, nil
}

func (m *mockHost) ChunkPut(_ context.Context, batchID, data []byte) (swarm.Address, error) {
	if m.err != nil {
		return swarm.ZeroAddress, m.err
	}
	if m.denyPut {
		return swarm.ZeroAddress, compute.ErrDenied
	}
	addr := addressOf(byte(len(m.chunks) + 100))
	m.chunks[addr.String()] = data
	return addr, nil
}

// addressOf builds a distinct, readable 32-byte address from a single byte.
func addressOf(seed byte) swarm.Address {
	b := make([]byte, swarm.HashSize)
	for i := range b {
		b[i] = seed
	}
	return swarm.NewAddress(b)
}

// u32 encodes a little-endian uint32 the way the fixtures read and write them.
func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// runHost executes a fixture against a host and returns its raw stdout.
func runHost(t *testing.T, host compute.Host, module string, input []byte, limits compute.Limits) compute.Result {
	t.Helper()

	s := newService(t, compute.Options{})
	res, err := s.Execute(t.Context(), compute.Request{
		Module: loadModule(t, module),
		Input:  input,
		Limits: limits,
		Host:   host,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res
}

// splitOutput cuts the leading fixed-width fields off a fixture's stdout.
func splitOutput(t *testing.T, out []byte, fields int) ([]uint32, []byte) {
	t.Helper()

	if len(out) < fields*4 {
		t.Fatalf("output too short: got %d bytes, want at least %d", len(out), fields*4)
	}
	values := make([]uint32, fields)
	for i := range values {
		values[i] = binary.LittleEndian.Uint32(out[i*4:])
	}
	return values, out[fields*4:]
}

func TestHostBytesGet(t *testing.T) {
	t.Parallel()

	payload := []byte("data reached the guest")

	for _, tc := range []struct {
		name      string
		addr      swarm.Address
		bufLen    uint32
		wantErrno uint32
		wantLen   uint32
		wantData  []byte
	}{
		{
			name:      "delivers the payload",
			addr:      addressOf(1),
			bufLen:    4096,
			wantErrno: errnoOK,
			wantLen:   uint32(len(payload)),
			wantData:  payload,
		},
		{
			name:      "missing address",
			addr:      addressOf(9),
			bufLen:    4096,
			wantErrno: errnoNotFound,
		},
		{
			name: "buffer too small reports the required length",
			addr: addressOf(1),
			// The probe half of the two-call pattern: no buffer at all, so the
			// guest learns how much to allocate before asking again.
			bufLen:    0,
			wantErrno: errnoBufferTooSmall,
			wantLen:   uint32(len(payload)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host := newMockHost()
			host.addData(1, payload)

			res := runHost(t, host, "hostbytesget", append(tc.addr.Bytes(), u32(tc.bufLen)...), compute.Limits{})
			if res.Status != compute.StatusOK {
				t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
			}

			fields, data := splitOutput(t, res.Output, 2)
			if fields[0] != tc.wantErrno {
				t.Errorf("errno: got %d, want %d", fields[0], tc.wantErrno)
			}
			if fields[1] != tc.wantLen {
				t.Errorf("required length: got %d, want %d", fields[1], tc.wantLen)
			}
			if !bytes.Equal(data, tc.wantData) {
				t.Errorf("payload: got %q, want %q", data, tc.wantData)
			}
		})
	}
}

func TestHostBytesPut(t *testing.T) {
	t.Parallel()

	payload := []byte("uploaded by the guest")
	batch := bytes.Repeat([]byte{7}, swarm.HashSize)

	t.Run("stores the payload", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		res := runHost(t, host, "hostbytesput", append(batch, payload...), compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
		}

		fields, ref := splitOutput(t, res.Output, 1)
		if fields[0] != errnoOK {
			t.Fatalf("errno: got %d, want %d", fields[0], errnoOK)
		}
		if len(ref) != swarm.HashSize {
			t.Fatalf("reference length: got %d, want %d", len(ref), swarm.HashSize)
		}
		if len(host.puts) != 1 || !bytes.Equal(host.puts[0], payload) {
			t.Fatalf("stored payload: got %q", host.puts)
		}
		// The reference must resolve to what was stored.
		if got := host.data[swarm.NewAddress(ref).String()]; !bytes.Equal(got, payload) {
			t.Errorf("reference resolves to %q, want %q", got, payload)
		}
	})

	t.Run("refused batch is a guest-visible verdict", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		host.denyPut = true

		res := runHost(t, host, "hostbytesput", append(batch, payload...), compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
		}
		fields, _ := splitOutput(t, res.Output, 1)
		if fields[0] != errnoDenied {
			t.Errorf("errno: got %d, want %d", fields[0], errnoDenied)
		}
	})
}

func TestHostChunkRoundTrip(t *testing.T) {
	t.Parallel()

	batch := bytes.Repeat([]byte{7}, swarm.HashSize)
	// An 8-byte span followed by the chunk payload, as the chunk API expects.
	chunk := append(u32(11), u32(0)...)
	chunk = append(chunk, []byte("chunk bytes")...)

	host := newMockHost()
	res := runHost(t, host, "hostchunk", append(batch, chunk...), compute.Limits{})
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
	}

	fields, data := splitOutput(t, res.Output, 2)
	if fields[0] != errnoOK {
		t.Fatalf("put errno: got %d, want %d", fields[0], errnoOK)
	}
	if fields[1] != errnoOK {
		t.Fatalf("get errno: got %d, want %d", fields[1], errnoOK)
	}
	if !bytes.Equal(data, chunk) {
		t.Errorf("retrieved chunk: got %q, want %q", data, chunk)
	}
}

func TestHostCallBudget(t *testing.T) {
	t.Parallel()

	host := newMockHost()
	addr := host.addData(1, []byte("x"))

	const allowed = 3
	res := runHost(t, host, "hostcalls", addr.Bytes(), compute.Limits{MaxHostCalls: allowed})
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
	}

	fields, _ := splitOutput(t, res.Output, 2)
	if fields[0] != allowed {
		t.Errorf("successful calls: got %d, want %d", fields[0], allowed)
	}
	if fields[1] != errnoBudgetExhausted {
		t.Errorf("errno: got %d, want %d", fields[1], errnoBudgetExhausted)
	}
	// The host must not have been asked to do work beyond the budget.
	if host.bytesGets != allowed {
		t.Errorf("host calls reaching the node: got %d, want %d", host.bytesGets, allowed)
	}
}

func TestHostByteBudget(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte("a"), 512)
	host := newMockHost()
	addr := host.addData(1, payload)

	// Enough for one delivery, not two.
	res := runHost(t, host, "hostcalls", addr.Bytes(), compute.Limits{MaxHostBytes: 600})
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
	}

	fields, _ := splitOutput(t, res.Output, 2)
	if fields[0] != 1 {
		t.Errorf("successful calls: got %d, want 1", fields[0])
	}
	if fields[1] != errnoBudgetExhausted {
		t.Errorf("errno: got %d, want %d", fields[1], errnoBudgetExhausted)
	}
}

func TestHostBadPointer(t *testing.T) {
	t.Parallel()

	// A pointer outside linear memory is the guest's mistake: it must come back
	// as a result code, not tear the module down.
	res := runHost(t, newMockHost(), "hostbadptr", nil, compute.Limits{})
	if res.Status != compute.StatusOK {
		t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
	}

	fields, _ := splitOutput(t, res.Output, 1)
	if fields[0] != errnoInvalid {
		t.Errorf("errno: got %d, want %d", fields[0], errnoInvalid)
	}
}

func TestHostErrorIsNeverATrap(t *testing.T) {
	t.Parallel()

	// The central invariant: a node-local failure ends the run as a host error
	// with a non-nil error. Reporting it as a trap would tell every caller the
	// program was at fault, which is a verdict this node has no right to make.
	host := newMockHost()
	host.addData(1, []byte("unreachable"))
	host.err = errors.New("storer exploded")

	s := newService(t, compute.Options{})
	res, err := s.Execute(t.Context(), compute.Request{
		Module: loadModule(t, "hostbytesget"),
		Input:  append(addressOf(1).Bytes(), u32(4096)...),
		Host:   host,
	})
	if err == nil {
		t.Fatal("expected a non-nil error for a node-local failure")
	}
	if res.Status != compute.StatusHostError {
		t.Errorf("status: got %v, want %v", res.Status, compute.StatusHostError)
	}
}

func TestHostUnavailable(t *testing.T) {
	t.Parallel()

	// With no Host the swarm module is not instantiated, so a module importing
	// it is rejected up front rather than trapping mid-run.
	s := newService(t, compute.Options{})
	res, err := s.Execute(t.Context(), compute.Request{
		Module: loadModule(t, "hostbytesget"),
		Input:  append(addressOf(1).Bytes(), u32(4096)...),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != compute.StatusInvalidModule {
		t.Errorf("status: got %v, want %v", res.Status, compute.StatusInvalidModule)
	}
}

func TestHostUnknownImport(t *testing.T) {
	t.Parallel()

	res := runHost(t, newMockHost(), "hostunknown", nil, compute.Limits{})
	if res.Status != compute.StatusInvalidModule {
		t.Errorf("status: got %v, want %v", res.Status, compute.StatusInvalidModule)
	}
}

func TestSwarmExportsMatchBuilder(t *testing.T) {
	t.Parallel()

	// checkImports rejects swarm imports outside the allowlist before the
	// module is instantiated. If the allowlist and the builder drift apart, a
	// real function becomes unreachable or a missing one becomes a link trap.
	defined, err := compute.SwarmModuleExports(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	allowed := make([]string, 0, len(compute.SwarmExports))
	for name := range compute.SwarmExports {
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)
	sort.Strings(defined)

	if len(allowed) != len(defined) {
		t.Fatalf("allowlist %v, host module defines %v", allowed, defined)
	}
	for i := range allowed {
		if allowed[i] != defined[i] {
			t.Errorf("allowlist %v, host module defines %v", allowed, defined)
			break
		}
	}
}

func TestHostNestedExecute(t *testing.T) {
	t.Parallel()

	t.Run("output of the nested module is forwarded", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		addr := host.addData(2, loadModule(t, "echo"))

		res := runHost(t, host, "hostnested", append(addr.Bytes(), []byte("nested input")...), compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
		}

		fields, data := splitOutput(t, res.Output, 2)
		if fields[0] != errnoOK {
			t.Fatalf("errno: got %d, want %d", fields[0], errnoOK)
		}
		if !bytes.Equal(data, []byte("nested input")) {
			t.Errorf("nested output: got %q, want %q", data, "nested input")
		}
	})

	t.Run("depth limit refuses nesting", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		addr := host.addData(2, loadModule(t, "echo"))

		// One level means the outermost execution and nothing below it.
		res := runHost(t, host, "hostnested", append(addr.Bytes(), []byte("x")...), compute.Limits{MaxDepth: 1})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v", res.Status, compute.StatusOK)
		}
		fields, _ := splitOutput(t, res.Output, 2)
		if fields[0] != errnoBudgetExhausted {
			t.Errorf("errno: got %d, want %d", fields[0], errnoBudgetExhausted)
		}
	})

	t.Run("a nested trap is a verdict, not a host failure", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		addr := host.addData(2, loadModule(t, "trap"))

		res := runHost(t, host, "hostnested", addr.Bytes(), compute.Limits{})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
		}
		fields, _ := splitOutput(t, res.Output, 2)
		if fields[0] != errnoExecFailed {
			t.Errorf("errno: got %d, want %d", fields[0], errnoExecFailed)
		}
	})

	t.Run("the call budget is shared with the nested module", func(t *testing.T) {
		t.Parallel()

		host := newMockHost()
		module := host.addData(2, loadModule(t, "hostcalls"))
		payload := host.addData(3, []byte("y"))

		// Three calls: swarm_execute takes one, leaving the nested module two
		// before it is cut off. Were the budget rebuilt per execution the
		// nested module would get all three.
		res := runHost(t, host, "hostnested", append(module.Bytes(), payload.Bytes()...), compute.Limits{MaxHostCalls: 3})
		if res.Status != compute.StatusOK {
			t.Fatalf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
		}

		fields, data := splitOutput(t, res.Output, 2)
		if fields[0] != errnoOK {
			t.Fatalf("errno: got %d, want %d", fields[0], errnoOK)
		}
		nested, _ := splitOutput(t, data, 2)
		if nested[0] != 2 {
			t.Errorf("nested successful calls: got %d, want 2", nested[0])
		}
		if nested[1] != errnoBudgetExhausted {
			t.Errorf("nested errno: got %d, want %d", nested[1], errnoBudgetExhausted)
		}
	})
}

// TestHostConcurrentExecutions runs many executions through one Service at once.
// Each host module closes over its own call's state, so a shared budget or a
// shared runtime would show up here as crossed results or a race.
func TestHostConcurrentExecutions(t *testing.T) {
	t.Parallel()

	s := newService(t, compute.Options{Workers: 8})
	module := loadModule(t, "hostbytesget")

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Every execution gets its own host, address and payload, so a
			// result that belongs to another run is visible immediately.
			seed := byte(i + 1)
			payload := bytes.Repeat([]byte{seed}, 64)
			host := newMockHost()
			addr := host.addData(seed, payload)

			res, err := s.Execute(t.Context(), compute.Request{
				Module: module,
				Input:  append(addr.Bytes(), u32(4096)...),
				Host:   host,
				// One call and one payload each: a budget shared between
				// executions would starve most of them.
				Limits: compute.Limits{MaxHostCalls: 1, MaxHostBytes: 64},
			})
			if err != nil {
				// The service refuses rather than queues when every worker is
				// taken, which is the point of the semaphore, not a failure.
				if !errors.Is(err, compute.ErrBusy) {
					t.Errorf("execute: %v", err)
				}
				return
			}
			if res.Status != compute.StatusOK {
				t.Errorf("status: got %v, want %v (%s)", res.Status, compute.StatusOK, res.TrapMessage)
				return
			}
			fields, data := splitOutput(t, res.Output, 2)
			if fields[0] != errnoOK {
				t.Errorf("errno: got %d, want %d", fields[0], errnoOK)
				return
			}
			if !bytes.Equal(data, payload) {
				t.Errorf("payload for seed %d: got %q", seed, data)
			}
		}()
	}
	wg.Wait()
}
