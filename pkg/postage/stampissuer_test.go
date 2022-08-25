// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

// TestStampIssuerMarshalling tests the idempotence  of binary marshal/unmarshal.
func TestStampIssuerMarshalling(t *testing.T) {
	st := newTestStampIssuer(t, 1000)
	buf, err := st.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	st0 := &postage.StampIssuer{}
	err = st0.UnmarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st, st0) {
		t.Fatalf("unmarshal(marshal(StampIssuer)) != StampIssuer \n%v\n%v", st, st0)
	}
}

func newTestStampIssuer(t *testing.T, block uint64) *postage.StampIssuer {
	t.Helper()
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	return postage.NewStampIssuer("label", "keyID", id, big.NewInt(3), 16, 8, block, true)
}

func TestGetUnexpiredStampIssuer(t *testing.T) {
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	si := postage.NewStampIssuer("label", "keyID", id, big.NewInt(3), 16, 8, 1, true)
	if si.Expired() != false {
		t.Fatalf(" stampIssuer.Expired() != false , required %v, is %v \n", false, si.Expired())
	}
}

func TestGetExpiredStampIssuer(t *testing.T) {
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	si := postage.NewStampIssuer("label", "keyID", id, big.NewInt(3), 16, 8, 1, true)
	si.SetExpired()
	if si.Expired() != true {
		t.Fatalf(" stampIssuer.Expired() != true , required %v, is %v \n", true, si.Expired())
	}
}
