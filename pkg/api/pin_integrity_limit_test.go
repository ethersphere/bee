// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

// TestPinIntegrityFullScanLimit guards against concurrent checks of all pins,
// each of which reads every pinned chunk from disk.
func TestPinIntegrityFullScanLimit(t *testing.T) {
	t.Parallel()

	pi := &blockingPinIntegrity{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	client, _, _, _, _ := newTestServer(t, testServerOptions{
		PinIntegrity: pi,
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/pins/check", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	select {
	case <-pi.started:
	case <-time.After(5 * time.Second):
		t.Fatal("check of all pins did not start")
	}

	// A second check of all pins is refused while the first runs.
	jsonhttptest.Request(t, client, http.MethodGet, "/pins/check", http.StatusTooManyRequests)

	// A check of a single pin is not affected.
	jsonhttptest.Request(t, client, http.MethodGet, "/pins/check?ref="+pinRef, http.StatusOK)

	// Once the first check ends another may start.
	close(pi.release)
	err = spinlock.Wait(5*time.Second, func() bool {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/pins/check", nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
	if err != nil {
		t.Fatal("check of all pins was not allowed after the previous one ended")
	}
}

// blockingPinIntegrity blocks checks of all pins until release is closed.
type blockingPinIntegrity struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (p *blockingPinIntegrity) Check(ctx context.Context, _ log.Logger, pin string, out chan storer.PinStat) {
	defer close(out)
	if pin != "" {
		return
	}
	p.once.Do(func() { p.started <- struct{}{} })
	select {
	case <-p.release:
	case <-ctx.Done():
	}
}
