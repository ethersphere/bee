// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func TestMarshal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []e // entries to add to manifest
	}{
		{
			name:    "empty-manifest",
			entries: nil,
		},
		{
			name: "one-entry",
			entries: []e{
				{
					reference: test.RandomAddress(),
					name:      "entry-1",
					headers:   http.Header{},
					path:      "",
				},
			},
		},
		{
			name: "two-entries",
			entries: []e{
				{
					reference: test.RandomAddress(),
					name:      "entry-1.txt",
					headers:   http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
					path:      "",
				},
				{
					reference: test.RandomAddress(),
					name:      "entry-2,png",
					headers:   http.Header{"Content-Type": {"image/png"}},
					path:      "",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := jsonmanifest.NewManifest()
			for _, e := range tc.entries {
				entry := jsonmanifest.NewEntry(
					e.reference,
					e.name,
					e.headers,
				)
				m.Add(e.path, entry)
			}

			b, err := m.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			um := jsonmanifest.NewManifest()

			if err := um.UnmarshalBinary(b); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(m, um) {
				t.Fatalf("marshalled and unmarshalled manifests are not equal: %v, %v", m, um)
			}
		})
	}
}

// struct for manifes entries for test cases
type e struct {
	reference swarm.Address
	name      string
	headers   http.Header
	path      string
}
