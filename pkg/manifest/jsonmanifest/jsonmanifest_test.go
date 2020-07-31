// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest_test

import (
	"net/http"
	"reflect"
	"testing"

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
		t.Fatal(err)
	}

	m2 := jsonmanifest.NewManifest()

	if err := m2.UnmarshalBinary(b); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(m1, m2) {
		t.Fatalf("marshalled and unmarshalled manifests are not equal: %v, %v", m1, m2)
	}
}
