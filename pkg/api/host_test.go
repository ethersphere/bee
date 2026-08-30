// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/compute"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Guest-visible result codes of the swarm host module, mirrored from the ABI.
const (
	hostErrnoOK     = 0
	hostErrnoDenied = 2
)

// loadHostFixture reads a WASM fixture from the compute package, which is where
// the guest ABI and its test modules are defined. See its testdata/README.md.
func loadHostFixture(t *testing.T, name string) []byte {
	t.Helper()

	module, err := os.ReadFile(filepath.Join("..", "compute", "testdata", name+".wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return module
}

// sessionRecorder notes how an upload session was finished. The mock storer
// stores puts in a shared chunk store and its Cleanup is a no-op, so committing
// and discarding look identical from the outside; this records which one the
// handler actually chose.
//
// It also holds the session to the context it was opened with. The real upload
// store batches its writes against that context, so a session opened on one
// that dies before the run is finished can never be committed; the mock ignores
// the context entirely, which is what let a cancelled-by-construction session
// pass every test here and fail on a node.
type sessionRecorder struct {
	storer.PutterSession
	ctx     context.Context
	done    *atomic.Bool
	cleaned *atomic.Bool
}

func (s sessionRecorder) Done(addr swarm.Address) error {
	s.done.Store(true)
	if err := s.ctx.Err(); err != nil {
		return err
	}
	return s.PutterSession.Done(addr)
}

func (s sessionRecorder) Cleanup() error {
	s.cleaned.Store(true)
	if err := s.ctx.Err(); err != nil {
		return err
	}
	return s.PutterSession.Cleanup()
}

// recordingStorer hands out recording upload sessions.
type recordingStorer struct {
	api.Storer
	done    atomic.Bool
	cleaned atomic.Bool
}

func (r *recordingStorer) Upload(ctx context.Context, pin bool, tagID uint64) (storer.PutterSession, error) {
	session, err := r.Storer.Upload(ctx, pin, tagID)
	if err != nil {
		return nil, err
	}
	return sessionRecorder{PutterSession: session, ctx: ctx, done: &r.done, cleaned: &r.cleaned}, nil
}

// newHostTestServer wires the real wazero engine behind the execute endpoint so
// the host calls exercise the actual storer and postage paths.
//
// issuerBatch is the only batch the node will stamp with; a guest passing any
// other must be refused rather than served.
func newHostTestServer(t *testing.T, issuerBatch []byte) *http.Client {
	t.Helper()

	client, _ := newHostTestServerWithStorer(t, issuerBatch)
	return client
}

func newHostTestServerWithStorer(t *testing.T, issuerBatch []byte) (*http.Client, *recordingStorer) {
	t.Helper()

	engine, err := compute.New(compute.Options{Logger: log.Noop})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close compute service: %v", err)
		}
	})

	store := &recordingStorer{Storer: mockstorer.New()}
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer:  store,
		Logger:  log.Noop,
		Post:    mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", issuerBatch, big.NewInt(3), 11, 10, 1000, true))),
		Compute: engine,
	})
	return client, store
}

// runModule uploads a fixture and executes it with the given input, returning
// the raw bytes the module wrote.
func runModule(t *testing.T, client *http.Client, fixture string, input []byte, wantStatus int, wantWasmStatus string) []byte {
	t.Helper()

	addr := uploadModule(t, client, loadHostFixture(t, fixture))

	var out []byte
	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), wantStatus,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
		jsonhttptest.WithRequestBody(bytes.NewReader(input)),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, wantWasmStatus),
		jsonhttptest.WithPutResponseBody(&out),
	)
	return out
}

// reset forgets the sessions used to upload the fixtures themselves, so a test
// observes only what the execution did.
func (r *recordingStorer) reset() {
	r.done.Store(false)
	r.cleaned.Store(false)
}

// hostErrno reads the leading result code a fixture writes.
func hostErrno(t *testing.T, out []byte) uint32 {
	t.Helper()

	if len(out) < 4 {
		t.Fatalf("output too short for a result code: %d bytes", len(out))
	}
	return binary.LittleEndian.Uint32(out)
}

// TestExecuteHostBytesGet uploads data through /bytes and has a module read it
// back through swarm_bytes_get.
func TestExecuteHostBytesGet(t *testing.T) {
	t.Parallel()

	client := newHostTestServer(t, batchOk)
	payload := []byte("data the guest reads back out of swarm")
	addr := uploadModule(t, client, payload)

	// stdin is the address followed by the buffer length the module offers.
	input := append(addr.Bytes(), u32le(4096)...)
	out := runModule(t, client, "hostbytesget", input, http.StatusOK, "ok")

	if errno := hostErrno(t, out); errno != hostErrnoOK {
		t.Fatalf("errno: got %d, want %d", errno, hostErrnoOK)
	}
	if got := out[8:]; !bytes.Equal(got, payload) {
		t.Errorf("payload: got %q, want %q", got, payload)
	}
}

// TestExecuteHostBytesPut has a module upload data and then fetches the
// reference it returned over /bytes, which only resolves if the deferred
// session was committed after the run.
func TestExecuteHostBytesPut(t *testing.T) {
	t.Parallel()

	client, store := newHostTestServerWithStorer(t, batchOk)
	payload := []byte("data the guest wrote into swarm")

	addr := uploadModule(t, client, loadHostFixture(t, "hostbytesput"))
	store.reset()

	var out []byte
	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
		jsonhttptest.WithRequestBody(bytes.NewReader(append(batchOk, payload...))),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
		jsonhttptest.WithPutResponseBody(&out),
	)

	if errno := hostErrno(t, out); errno != hostErrnoOK {
		t.Fatalf("errno: got %d, want %d", errno, hostErrnoOK)
	}
	if !store.done.Load() {
		t.Error("upload session was not committed after a clean run")
	}
	if store.cleaned.Load() {
		t.Error("upload session was discarded after a clean run")
	}

	ref := swarm.NewAddress(out[4:])
	if len(out[4:]) != swarm.HashSize {
		t.Fatalf("reference length: got %d, want %d", len(out[4:]), swarm.HashSize)
	}

	jsonhttptest.Request(t, client, http.MethodGet, "/bytes/"+ref.String(), http.StatusOK,
		jsonhttptest.WithExpectedResponse(payload),
	)
}

// TestExecuteHostBadBatch checks that an unusable batch is reported to the
// module as a result code, not surfaced as a node failure.
func TestExecuteHostBadBatch(t *testing.T) {
	t.Parallel()

	client := newHostTestServer(t, batchOk)

	// A batch the node issues nothing for. The run itself is fine; only the
	// upload is refused, so the endpoint answers 200 and the module reports it.
	otherBatch := bytes.Repeat([]byte{9}, swarm.HashSize)
	out := runModule(t, client, "hostbytesput", append(otherBatch, []byte("payload")...), http.StatusOK, "ok")

	if errno := hostErrno(t, out); errno != hostErrnoDenied {
		t.Errorf("errno: got %d, want %d", errno, hostErrnoDenied)
	}
}

// TestExecuteHostTrapDiscardsUpload checks that a module which uploads and then
// traps leaves nothing behind: its chunks are dropped rather than handed to the
// pusher.
func TestExecuteHostTrapDiscardsUpload(t *testing.T) {
	t.Parallel()

	client, store := newHostTestServerWithStorer(t, batchOk)
	payload := []byte("data that must not survive the trap")
	addr := uploadModule(t, client, loadHostFixture(t, "hostputtrap"))
	store.reset()

	// A trap is a program verdict, so the endpoint answers 400. The JSON
	// envelope still carries what the module wrote before trapping, which
	// includes the reference its upload returned.
	var resp struct {
		Status string `json:"status"`
		Output []byte `json:"output"`
	}
	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+addr.String(), http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/json"),
		jsonhttptest.WithRequestBody(bytes.NewReader(append(batchOk, payload...))),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "trap"),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
	)

	if errno := hostErrno(t, resp.Output); errno != hostErrnoOK {
		t.Fatalf("errno: got %d, want %d", errno, hostErrnoOK)
	}
	if len(resp.Output[4:]) != swarm.HashSize {
		t.Fatalf("reference length: got %d, want %d", len(resp.Output[4:]), swarm.HashSize)
	}

	if store.done.Load() {
		t.Error("upload session was committed after a trapped run")
	}
	if !store.cleaned.Load() {
		t.Error("upload session was not discarded after a trapped run")
	}
}

// u32le encodes a little-endian uint32 the way the fixtures read them.
func u32le(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// TestExecuteHostByteLimit checks that an object larger than the execution's
// byte budget is refused by the node before it is read into memory, and that
// the module sees that as a result code rather than a failure.
func TestExecuteHostByteLimit(t *testing.T) {
	t.Parallel()

	const hostErrnoBudgetExhausted = 3

	client := newHostTestServer(t, batchOk)
	payload := bytes.Repeat([]byte("x"), 4096)
	target := uploadModule(t, client, payload)
	module := uploadModule(t, client, loadHostFixture(t, "hostbytesget"))

	var out []byte
	jsonhttptest.Request(t, client, http.MethodPost, "/@/"+module.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader(api.AcceptHeader, "application/octet-stream"),
		// Far less than the payload, so the download is refused up front.
		jsonhttptest.WithRequestHeader(api.SwarmWasmHostBytesHeader, "64"),
		jsonhttptest.WithRequestBody(bytes.NewReader(append(target.Bytes(), u32le(4096)...))),
		jsonhttptest.WithExpectedResponseHeader(api.SwarmWasmStatusHeader, "ok"),
		jsonhttptest.WithPutResponseBody(&out),
	)

	if errno := hostErrno(t, out); errno != hostErrnoBudgetExhausted {
		t.Errorf("errno: got %d, want %d", errno, hostErrnoBudgetExhausted)
	}
}
