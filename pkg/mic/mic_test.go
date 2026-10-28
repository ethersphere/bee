// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mic_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/mic"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// TestRegister verifies that handler funcs are registered and matched by SOC owner,
// independently of the SOC id.
func TestRegister(t *testing.T) {
	t.Parallel()

	var (
		g       = mic.New(log.Noop)
		h1Calls = 0
		h2Calls = 0
		h3Calls = 0
		msgChan = make(chan struct{})

		payload1 = []byte("Hello there!")
		payload2 = []byte("General Kenobi. You are a bold one. Kill him!")

		privKey1, _ = crypto.GenerateSecp256k1Key()
		signer1     = crypto.NewDefaultSigner(privKey1)
		owner1, _   = signer1.EthereumAddress()
		privKey2, _ = crypto.GenerateSecp256k1Key()
		signer2     = crypto.NewDefaultSigner(privKey2)
		owner2, _   = signer2.EthereumAddress()

		socId1 = testutil.RandBytes(t, 32)
		socId2 = append([]byte{socId1[0] + 1}, socId1[1:]...)

		h1 = func(m []byte) {
			h1Calls++
			msgChan <- struct{}{}
		}

		h2 = func(m []byte) {
			h2Calls++
			msgChan <- struct{}{}
		}

		h3 = func(m []byte) {
			h3Calls++
			msgChan <- struct{}{}
		}
	)
	_ = g.Subscribe(owner1.Bytes(), h1)
	_ = g.Subscribe(owner2.Bytes(), h2)

	// soc with socId1 signed by signer1 (owner1)
	ch1, _ := cac.New(payload1)
	socCh1 := soc.New(socId1, ch1)
	ch1, _ = socCh1.Sign(signer1)
	socCh1, _ = soc.FromChunk(ch1)

	// soc with socId1 signed by signer2 (owner2)
	ch2, _ := cac.New(payload2)
	socCh2 := soc.New(socId1, ch2)
	ch2, _ = socCh2.Sign(signer2)
	socCh2, _ = soc.FromChunk(ch2)

	// soc with a different id (socId2) but same owner1: must still match h1
	ch3, _ := cac.New(payload1)
	socCh3 := soc.New(socId2, ch3)
	ch3, _ = socCh3.Sign(signer1)
	socCh3, _ = soc.FromChunk(ch3)

	// trigger soc from owner1, check that only h1 is called
	g.Handle(socCh1)
	waitHandlerCallback(t, &msgChan, 1)
	ensureCalls(t, &h1Calls, 1)
	ensureCalls(t, &h2Calls, 0)

	// register another handler on owner1
	cleanup := g.Subscribe(owner1.Bytes(), h3)
	g.Handle(socCh1)
	waitHandlerCallback(t, &msgChan, 2)
	ensureCalls(t, &h1Calls, 2)
	ensureCalls(t, &h2Calls, 0)
	ensureCalls(t, &h3Calls, 1)

	cleanup() // remove the last handler
	g.Handle(socCh1)
	waitHandlerCallback(t, &msgChan, 1)
	ensureCalls(t, &h1Calls, 3)
	ensureCalls(t, &h2Calls, 0)
	ensureCalls(t, &h3Calls, 1)

	// different owner only triggers its own handler
	g.Handle(socCh2)
	waitHandlerCallback(t, &msgChan, 1)
	ensureCalls(t, &h1Calls, 3)
	ensureCalls(t, &h2Calls, 1)
	ensureCalls(t, &h3Calls, 1)

	// same owner, different id still matches the owner handler
	g.Handle(socCh3)
	waitHandlerCallback(t, &msgChan, 1)
	ensureCalls(t, &h1Calls, 4)
	ensureCalls(t, &h2Calls, 1)
	ensureCalls(t, &h3Calls, 1)
}

func ensureCalls(t *testing.T, calls *int, exp int) {
	t.Helper()

	if exp != *calls {
		t.Fatalf("expected %d calls, found %d", exp, *calls)
	}
}

func waitHandlerCallback(t *testing.T, msgChan *chan struct{}, count int) {
	t.Helper()

	for range count {
		select {
		case <-*msgChan:
		case <-time.After(1 * time.Second):
			t.Fatal("reached timeout while waiting for handler message")
		}
	}
}
