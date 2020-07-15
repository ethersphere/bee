// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trojan_test

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

// arbitrary targets for tests
var t1 = trojan.Target([]byte{57, 120})
var t2 = trojan.Target([]byte{209, 156})
var t3 = trojan.Target([]byte{156, 38})
var t4 = trojan.Target([]byte{89, 19})
var t5 = trojan.Target([]byte{22, 129})
var testTargets = trojan.Targets([]trojan.Target{t1, t2, t3, t4, t5})

// arbitrary topic for tests
var testTopic = trojan.NewTopic("foo")

// newTestMessage creates an arbitrary Message for tests
func newTestMessage(t *testing.T) trojan.Message {
	payload := []byte("foopayload")
	m, err := trojan.NewMessage(testTopic, payload)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

// TestNewMessage tests the correct and incorrect creation of a Message struct
func TestNewMessage(t *testing.T) {
	smallPayload := make([]byte, 32)
	m, err := trojan.NewMessage(testTopic, smallPayload)
	if err != nil {
		t.Fatal(err)
	}

	// verify topic
	if m.Topic != testTopic {
		t.Fatalf("expected message topic to be %v but is %v instead", testTopic, m.Topic)
	}

	maxPayload := make([]byte, trojan.MaxPayloadSize)
	if _, err := trojan.NewMessage(testTopic, maxPayload); err != nil {
		t.Fatal(err)
	}

	// the creation should fail if the payload is too big
	invalidPayload := make([]byte, trojan.MaxPayloadSize+1)
	if _, err := trojan.NewMessage(testTopic, invalidPayload); err != trojan.ErrPayloadTooBig {
		t.Fatalf("expected error when creating message of invalid payload size to be %q, but got %v", trojan.ErrPayloadTooBig, err)
	}
}

// TestWrap tests the creation of a chunk from a list of targets
// its address length and span should be correct
// its resulting address should have a prefix which matches one of the given targets
// its resulting data should have a hash that matches its address exactly
func TestWrap(t *testing.T) {
	m := newTestMessage(t)
	c, err := m.Wrap(testTargets)
	if err != nil {
		t.Fatal(err)
	}

	addr := c.Address()
	addrLen := len(addr.Bytes())
	if addrLen != swarm.HashSize {
		t.Fatalf("chunk has an unexpected address length of %d rather than %d", addrLen, swarm.HashSize)
	}

	addrPrefix := addr.Bytes()[:len(testTargets[0])]
	if !trojan.Contains(testTargets, addrPrefix) {
		t.Fatal("chunk address prefix does not match any of the targets")
	}

	data := c.Data()
	dataSize := len(data)
	expectedSize := file.ChunkWithSpanSize // span + payload
	if dataSize != expectedSize {
		t.Fatalf("chunk data has an unexpected size of %d rather than %d", dataSize, expectedSize)
	}

	span := binary.LittleEndian.Uint64(data[:8])
	remainingDataLen := len(data[8:])
	if int(span) != remainingDataLen {
		t.Fatalf("chunk span set to %d, but rest of chunk data is of size %d", span, remainingDataLen)
	}

	dataHash, err := trojan.HashBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(addr.Bytes(), dataHash) {
		t.Fatal("chunk address does not match its data hash")
	}
}

// TestWrapFail tests that the creation of a chunk fails when given targets are invalid
func TestWrapFail(t *testing.T) {
	m := newTestMessage(t)

	emptyTargets := trojan.Targets([]trojan.Target{})
	if _, err := m.Wrap(emptyTargets); err != trojan.ErrEmptyTargets {
		t.Fatalf("expected error when creating chunk for empty targets to be %q, but got %v", trojan.ErrEmptyTargets, err)
	}

	t1 := trojan.Target([]byte{34})
	t2 := trojan.Target([]byte{25, 120})
	t3 := trojan.Target([]byte{180, 18, 255})
	varLenTargets := trojan.Targets([]trojan.Target{t1, t2, t3})
	if _, err := m.Wrap(varLenTargets); err != trojan.ErrVarLenTargets {
		t.Fatalf("expected error when creating chunk for variable-length targets to be %q, but got %v", trojan.ErrVarLenTargets, err)
	}
}

// TestPadBytes tests that different types of byte slices are correctly padded with leading 0s
// all slices are interpreted as big-endian
func TestPadBytes(t *testing.T) {
	s := make([]byte, 32)

	// empty slice should be unchanged
	p := trojan.PadBytes(s)
	if !bytes.Equal(p, s) {
		t.Fatalf("expected byte padding to result in %x, but is %x", s, p)
	}

	// slice of length 3
	s = []byte{255, 128, 64}
	p = trojan.PadBytes(s)
	e := append(make([]byte, 29), s...) // 29 zeros plus the 3 original bytes
	if !bytes.Equal(p, e) {
		t.Fatalf("expected byte padding to result in %x, but is %x", e, p)
	}

	// simulate toChunk behavior
	s = []byte{1, 0, 0, 0}

	p = trojan.PadBytes(s)
	e = append(make([]byte, 28), s...) // 28 zeros plus the 4 original bytes
	if !bytes.Equal(p, e) {
		t.Fatalf("expected byte padding to result in %x, but is %x", e, p)
	}
}

// TestUnwrap tests the correct unwrapping of a trojan chunk to obtain a message
func TestUnwrap(t *testing.T) {
	m := newTestMessage(t)
	c, err := m.Wrap(testTargets)
	if err != nil {
		t.Fatal(err)
	}

	um, err := trojan.Unwrap(c)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(m, *um) {
		t.Fatalf("original message does not match unwrapped one")
	}
}

// TestIsPotential tests if chunks are correctly interpreted as potentially trojan
func TestIsPotential(t *testing.T) {
	c := chunktesting.GenerateTestRandomChunk()

	// invalid type
	c.WithType(swarm.Unknown)
	if trojan.IsPotential(c) {
		t.Fatal("non content-addressed chunk marked as potential trojan")
	}

	// valid type, but invalid trojan message length
	c.WithType(swarm.ContentAddressed)
	length := len(c.Data()) - 73 // go 1 byte over the maximum allowed
	lengthBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBuf, uint16(length))
	// put invalid length into bytes #41 and #42
	copy(c.Data()[40:42], lengthBuf)
	if trojan.IsPotential(c) {
		t.Fatal("chunk with invalid trojan message length marked as potential trojan")
	}

	// valid type, but invalid chunk data length
	data := make([]byte, 10)
	c = swarm.NewChunk(swarm.ZeroAddress, data)
	c.WithType(swarm.ContentAddressed)
	if trojan.IsPotential(c) {
		t.Fatal("chunk with invalid data length marked as potential trojan")
	}

	// valid potential trojan
	m := newTestMessage(t)
	c, err := m.Wrap(testTargets)
	if err != nil {
		t.Fatal(err)
	}
	c.WithType(swarm.ContentAddressed)
	if !trojan.IsPotential(c) {
		t.Fatal("valid test trojan chunk not marked as potential trojan")
	}
}

// TestMessageSerialization tests that the Message type can be correctly serialized and deserialized
func TestMessageSerialization(t *testing.T) {
	m := newTestMessage(t)

	sm, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dsm := new(trojan.Message)
	err = dsm.UnmarshalBinary(sm)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(m, *dsm) {
		t.Fatalf("original message does not match deserialized one")
	}
}
