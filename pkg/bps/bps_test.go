// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestClaimAndBroadcast(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	brokerAddr := swarm.RandAddress(t)
	broker := bps.New(nil, brokerAddr, true, logger)

	// each client gets its own recorder so that the broker sees distinct peer overlays
	newClient := func() *bps.Service {
		recorder := streamtest.New(
			streamtest.WithProtocols(broker.Protocol()),
			streamtest.WithBaseAddr(swarm.RandAddress(t)),
		)
		return bps.New(recorder, swarm.RandAddress(t), false, logger)
	}
	publisher := newClient()
	subscriber := newClient()

	// the public topic is the soc address of the claim: keccak(id | owner)
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := crypto.NewEthereumAddress(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, swarm.HashSize)
	copy(id, "bps-test-topic")
	topic, err := soc.CreateAddress(id, owner)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, subRx, _, _, err := subscriber.Join(ctx, brokerAddr, topic.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	challenge, pubRx, pubTx, claim, err := publisher.Join(ctx, brokerAddr, topic.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// claim payload: challenge | broker overlay
	payload := append(append([]byte{}, challenge...), brokerAddr.Bytes()...)
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	claim(proof.Data())

	msg := []byte("hello cohort")
	select {
	case pubTx <- msg:
	case <-time.After(time.Second):
		t.Fatal("timed out sending broadcast")
	}

	select {
	case got := <-subRx:
		if !bytes.Equal(got, msg) {
			t.Fatalf("got message %q, want %q", got, msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber to receive broadcast")
	}

	select {
	case got := <-pubRx:
		t.Fatalf("publisher received its own broadcast %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}
