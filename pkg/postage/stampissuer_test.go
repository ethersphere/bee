// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"io"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

// TestStampIssuerMarshalling tests the idempotence  of binary marshal/unmarshal.
func TestStampIssuerMarshalling(t *testing.T) {
	st := newTestStampIssuer(t)
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

func newTestStampIssuer(t *testing.T) *postage.StampIssuer {
	t.Helper()
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	return postage.NewStampIssuer("label", "keyID", id, 12, 8)
}
