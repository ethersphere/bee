// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestWrap(t *testing.T) {
	topic := pss.NewTopic("topic")
	msg := []byte("some payload")
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	pubkey := &key.PublicKey
	depth := 1
	targets := newTargets(4, depth)

	chunk, err := pss.Wrap(context.Background(), topic, msg, pubkey, targets)
	if err != nil {
		t.Fatal(err)
	}

	contains := pss.Contains(targets, chunk.Address().Bytes()[0:depth])
	if !contains {
		t.Fatal("trojan address was expected to match one of the targets with prefix")
	}

	if len(chunk.Data()) != swarm.ChunkWithSpanSize {
		t.Fatalf("expected trojan data size to be %d, was %d", swarm.ChunkWithSpanSize, len(chunk.Data()))
	}
}

func TestUnwrap(t *testing.T) {
	topic := pss.NewTopic("topic")
	msg := []byte("some payload")
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	pubkey := &key.PublicKey
	depth := 1
	targets := newTargets(4, depth)

	chunk, err := pss.Wrap(context.Background(), topic, msg, pubkey, targets)
	if err != nil {
		t.Fatal(err)
	}

	topic1 := pss.NewTopic("topic-1")
	topic2 := pss.NewTopic("topic-2")

	unwrapTopic, unwrapMsg, err := pss.Unwrap(context.Background(), key, chunk, []pss.Topic{topic1, topic2, topic})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(msg, unwrapMsg) {
		t.Fatalf("message mismatch: expected %x, got %x", msg, unwrapMsg)
	}

	if !bytes.Equal(topic[:], unwrapTopic[:]) {
		t.Fatalf("topic mismatch: expected %x, got %x", topic[:], unwrapTopic[:])
	}
}

func TestUnwrapTopicEncrypted(t *testing.T) {
	topic := pss.NewTopic("topic")
	msg := []byte("some payload")

	privk := crypto.Secp256k1PrivateKeyFromBytes(topic[:])
	pubkey := privk.PublicKey

	depth := 1
	targets := newTargets(4, depth)

	chunk, err := pss.Wrap(context.Background(), topic, msg, &pubkey, targets)
	if err != nil {
		t.Fatal(err)
	}

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	topic1 := pss.NewTopic("topic-1")
	topic2 := pss.NewTopic("topic-2")

	unwrapTopic, unwrapMsg, err := pss.Unwrap(context.Background(), key, chunk, []pss.Topic{topic1, topic2, topic})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(msg, unwrapMsg) {
		t.Fatalf("message mismatch: expected %x, got %x", msg, unwrapMsg)
	}

	if !bytes.Equal(topic[:], unwrapTopic[:]) {
		t.Fatalf("topic mismatch: expected %x, got %x", topic[:], unwrapTopic[:])
	}
}
