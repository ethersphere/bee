// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/updatecheck"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const registryURL = "https://registry.example"

var (
	testSigner  = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	testPubHex  = hex.EncodeToString(testSigner.Public().(ed25519.PublicKey))
	otherSigner = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize))
	otherPubHex = hex.EncodeToString(otherSigner.Public().(ed25519.PublicKey))
)

// release is what the registry offers.
type release struct {
	version   uint64
	channels  []string
	tags      []string
	notes     string
	createdAt time.Time
	window    *time.Duration
}

func window(d time.Duration) *time.Duration { return &d }

// descriptor renders r as a signed release descriptor, as swarm-oci-publish
// does.
func descriptor(t *testing.T, r release) []byte {
	t.Helper()
	d := map[string]any{
		"schemaVersion": 2,
		"repository":    "ethersphere/bee",
		"version":       r.version,
		"files":         map[string]string{},
	}
	if len(r.channels) > 0 {
		d["channels"] = r.channels
	}
	if r.notes != "" {
		d["notes"] = r.notes
	}
	if !r.createdAt.IsZero() {
		d["createdAt"] = r.createdAt.UTC().Format(time.RFC3339)
	}
	if r.window != nil {
		d["rolloutWindowSeconds"] = uint64(*r.window / time.Second)
	}
	tags := map[string]string{}
	for _, tag := range r.tags {
		tags[tag] = "sha256:" + strings.Repeat("0", 64)
	}
	d["tags"] = tags
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type testSig struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

func sig(key ed25519.PrivateKey, body []byte) testSig {
	return testSig{
		KeyID: hex.EncodeToString(key.Public().(ed25519.PublicKey)),
		Sig:   hex.EncodeToString(ed25519.Sign(key, body)),
	}
}

func sigSet(t *testing.T, sigs ...testSig) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"signatures": sigs})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// info renders r as the registry's /info summary.
func info(t *testing.T, r release, verified bool) []byte {
	t.Helper()
	type tag struct {
		Tag string `json:"tag"`
	}
	i := map[string]any{"verified": verified, "version": r.version, "channels": r.channels}
	tags := make([]tag, 0, len(r.tags))
	for _, tg := range r.tags {
		tags = append(tags, tag{tg})
	}
	i["tags"] = tags
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// response is what the fake registry serves for a path.
type response struct {
	status int
	body   []byte
	etag   string
}

// registry is an in-memory swarm-oci-serve, served through an
// http.RoundTripper so that tests run in a synctest bubble.
type registry struct {
	mu        sync.Mutex
	responses map[string]response
	requests  map[string][]*http.Request
	times     map[string][]time.Time
}

func newRegistry() *registry {
	return &registry{
		responses: map[string]response{},
		requests:  map[string][]*http.Request{},
		times:     map[string][]time.Time{},
	}
}

func (r *registry) client() *http.Client { return &http.Client{Transport: r} }

func (r *registry) set(path string, resp response) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if resp.status == 0 {
		resp.status = http.StatusOK
	}
	r.responses[path] = resp
}

// offer serves r signed by the test key, and as a verified /info.
func (r *registry) offer(t *testing.T, rel release) {
	t.Helper()
	body := descriptor(t, rel)
	r.set("/release.json", response{body: body})
	r.set("/release.sig", response{body: sigSet(t, sig(testSigner, body))})
	r.set("/info", response{body: info(t, rel, true)})
}

func (r *registry) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests[path])
}

func (r *registry) requestTimes(path string) []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.times[path]...)
}

func (r *registry) lastRequest(path string) *http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	reqs := r.requests[path]
	if len(reqs) == 0 {
		return nil
	}
	return reqs[len(reqs)-1]
}

func (r *registry) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests[req.URL.Path] = append(r.requests[req.URL.Path], req)
	r.times[req.URL.Path] = append(r.times[req.URL.Path], time.Now())

	resp, ok := r.responses[req.URL.Path]
	if !ok {
		resp = response{status: http.StatusNotFound}
	}
	header := http.Header{}
	if resp.etag != "" {
		header.Set("ETag", resp.etag)
		if req.Header.Get("If-None-Match") == resp.etag {
			resp = response{status: http.StatusNotModified}
		}
	}
	return &http.Response{
		StatusCode: resp.status,
		// A status line a hostile registry controls; it must not be logged.
		Status:  "999 status from the registry",
		Header:  header,
		Body:    io.NopCloser(bytes.NewReader(resp.body)),
		Request: req,
	}, nil
}

// logs returns a logger that writes everything to the returned buffer.
func logs() (log.Logger, *syncBuffer) {
	var b syncBuffer
	return log.NewLogger("test", log.WithSink(&b), log.WithVerbosity(log.VerbosityAll)), &b
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// runnerOptions are options as bee-runner hands them over with the test key,
// running release version 1000.
func runnerOptions(reg *registry) updatecheck.Options {
	return updatecheck.Options{
		URL:            registryURL,
		CurrentVersion: "2.8.0-7e703f49",
		Client:         reg.client(),
		Runner: updatecheck.Runner{
			Started:  true,
			Registry: registryURL,
			Channel:  "stable",
			Version:  "1000",
			Pubkey:   testPubHex,
		},
	}
}
