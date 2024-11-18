// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gsoc_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	t.Parallel()

	var (
		g       = gsoc.New(log.Noop)
		h1Calls = 0
		h2Calls = 0
		h3Calls = 0
		msgChan = make(chan struct{})

		payload1    = []byte("Hello there!")
		payload2    = []byte("General Kenobi. You are a bold one. Kill him!")
		socId1      = testutil.RandBytes(t, 32)
		socId2      = append([]byte{socId1[0] + 1}, socId1[1:]...)
		privKey, _  = crypto.GenerateSecp256k1Key()
		signer      = crypto.NewDefaultSigner(privKey)
		owner, _    = signer.EthereumAddress()
		address1, _ = soc.CreateAddress(socId1, owner.Bytes())
		address2, _ = soc.CreateAddress(socId2, owner.Bytes())

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
	_ = g.Subscribe(address1, h1)
	_ = g.Subscribe(address2, h2)

	ch1, _ := cac.New(payload1)
	socCh1 := soc.New(socId1, ch1)
	ch1, _ = socCh1.Sign(signer)
	socCh1, _ = soc.FromChunk(ch1)

	ch2, _ := cac.New(payload2)
	socCh2 := soc.New(socId2, ch2)
	ch2, _ = socCh2.Sign(signer)
	socCh2, _ = soc.FromChunk(ch2)

	// trigger soc upload on address1, check that only h1 is called
	g.Handle(socCh1)

	waitHandlerCallback(t, &msgChan, 1)

	ensureCalls(t, &h1Calls, 1)
	ensureCalls(t, &h2Calls, 0)

	// register another handler on the first address
	cleanup := g.Subscribe(address1, h3)

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

	g.Handle(socCh2)

	waitHandlerCallback(t, &msgChan, 1)

	ensureCalls(t, &h1Calls, 3)
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

	for received := 0; received < count; received++ {
		select {
		case <-*msgChan:
		case <-time.After(1 * time.Second):
			t.Fatal("reached timeout while waiting for handler message")
		}
	}
}
