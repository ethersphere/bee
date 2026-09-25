// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/moc"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/gorilla/websocket"
)

// TestMocWebsocketSingleHandler subscribes on a SOC id and receives a message.
func TestMocWebsocketSingleHandler(t *testing.T) {
	t.Parallel()

	var (
		id            = make([]byte, 32)
		m, cl, signer = newMocTest(t, id, 0)
		respC         = make(chan error, 1)
		payload       = []byte("hello there!")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)
	m.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func newMocTest(t *testing.T, socId []byte, pingPeriod time.Duration) (moc.Listener, *websocket.Conn, crypto.Signer) {
	t.Helper()
	if pingPeriod == 0 {
		pingPeriod = 10 * time.Second
	}
	var (
		batchStore = mockbatchstore.New()
		storer     = mockstorer.New()
	)

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	mocService := moc.New(log.NewLogger("test"))
	testutil.CleanupCloser(t, mocService)

	_, cl, _, _ := newTestServer(t, testServerOptions{
		Moc:          mocService,
		WsPath:       fmt.Sprintf("/moc/subscribe/%s", hex.EncodeToString(socId)),
		Storer:       storer,
		BatchStore:   batchStore,
		Logger:       log.Noop,
		WsPingPeriod: pingPeriod,
	})

	return mocService, cl, signer
}
