// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/gorilla/websocket"
)

// TestGsocWebsocketSingleHandler creates a single websocket handler on a chunk address, and receives a message
func TestGsocWebsocketSingleHandler(t *testing.T) {
	t.Parallel()

	var (
		id               = make([]byte, 32)
		g, cl, signer, _ = newGsocTest(t, id, 0)
		respC            = make(chan error, 1)
		payload          = []byte("hello there!")
	)

	err := cl.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)
	g.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func TestGsocWebsocketMultiHandler(t *testing.T) {
	t.Parallel()

	var (
		id                      = make([]byte, 32)
		g, cl, signer, listener = newGsocTest(t, make([]byte, 32), 0)
		owner, _                = signer.EthereumAddress()
		chunkAddr, _            = soc.CreateAddress(id, owner.Bytes())
		u                       = url.URL{Scheme: "ws", Host: listener, Path: fmt.Sprintf("/gsoc/subscribe/%s", hex.EncodeToString(chunkAddr.Bytes()))}
		cl2, _, err             = websocket.DefaultDialer.Dial(u.String(), nil)
		respC                   = make(chan error, 2)
	)
	if err != nil {
		t.Fatalf("dial: %v. url %v", err, u.String())
	}
	testutil.CleanupCloser(t, cl2)

	err = cl.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)

	// close the websocket before calling GSOC with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	g.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	go expectMessage(t, cl2, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// TestGsocPong tests that the websocket api adheres to the websocket standard
// and sends ping-pong messages to keep the connection alive.
// The test opens a websocket, keeps it alive for 500ms, then receives a GSOC message.
func TestGsocPong(t *testing.T) {
	t.Parallel()
	id := make([]byte, 32)

	var (
		g, cl, signer, _ = newGsocTest(t, id, 90*time.Millisecond)

		respC    = make(chan error, 1)
		pongWait = 1 * time.Millisecond
	)

	cl.SetReadLimit(swarm.ChunkSize)
	err := cl.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // wait to see that the websocket is kept alive
	ch, _ := cac.New([]byte("hello there!"))
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)

	g.Handle(socCh)

	go expectMessage(t, cl, respC, nil)
	if err := <-respC; err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		// note: error has *websocket.netError type so we need to check error by checking message
		t.Fatal("want timeout error")
	}
}

func newGsocTest(t *testing.T, socId []byte, pingPeriod time.Duration) (gsoc.Listener, *websocket.Conn, crypto.Signer, string) {
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
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	chunkAddr, _ := soc.CreateAddress(socId, owner.Bytes())

	gsoc := gsoc.New(log.NewLogger("test"))
	testutil.CleanupCloser(t, gsoc)

	_, cl, listener, _ := newTestServer(t, testServerOptions{
		Gsoc:         gsoc,
		WsPath:       fmt.Sprintf("/gsoc/subscribe/%s", hex.EncodeToString(chunkAddr.Bytes())),
		Storer:       storer,
		BatchStore:   batchStore,
		Logger:       log.Noop,
		WsPingPeriod: pingPeriod,
	})

	return gsoc, cl, signer, listener
}
