// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func TestMarshal(t *testing.T) {
	m1 := jsonmanifest.NewManifest()

	e1 := jsonmanifest.NewEntry(
		test.RandomAddress(),
		"entry-1",
		http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
	)

	m1.Add("entry-1", e1)

	e2 := jsonmanifest.NewEntry(
		test.RandomAddress(),
		"entry-2",
		http.Header{"Content-Type": {"image/png"}},
	)

	m1.Add("entry-2", e2)

	b, err := m1.MarshalBinary()
	if err != nil {
		t.Fatal("error marshalling")
	}

	m2 := jsonmanifest.NewManifest()

	if err := m2.UnmarshalBinary(b); err != nil {
		t.Fatal("error unmarshalling")
	}
}

func verifyEquals(t *testing.T, e1, e2 manifest.Entry) {
	r1 := e1.Reference()
	r2 := e2.Reference()
	if !r1.Equal(r2) {
		t.Fatalf("entry references are not equal: %v, %v", r1, r2)
	}

	n1 := e1.Name()
	n2 := e2.Name()
	if n1 != n2 {
		t.Fatalf("entry names are not equal: %v, %v", n1, n2)
	}

	h1 := e1.Headers()
	h2 := e2.Headers()
	if !reflect.DeepEqual(h1, h2) {
		t.Fatalf("entry headers are not equal: %v, %v", h1, h2)
	}
}
