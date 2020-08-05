// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest_test

import (
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

var testCases = []testCase{
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
				name:      "entry-2.png",
				headers:   http.Header{"Content-Type": {"image/png"}},
				path:      "",
			},
		},
	},
	{
		name: "nested-entries",
		entries: []e{
			{
				reference: test.RandomAddress(),
				name:      "robots.txt",
				headers:   http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
				path:      "text",
			},
			{
				reference: test.RandomAddress(),
				name:      "1.png",
				headers:   http.Header{"Content-Type": {"image/png"}},
				path:      "img",
			},
			{
				reference: test.RandomAddress(),
				name:      "2.jpg",
				headers:   http.Header{"Content-Type": {"image/jpg"}},
				path:      "img",
			},
			{
				reference: test.RandomAddress(),
				name:      "readme.md",
				headers:   http.Header{"Content-Type": {"text/markdown; charset=UTF-8"}},
				path:      "",
			},
		},
	},
}

// TestEntries verifies that manifests behave as expected when adding and removing entries.
// It also tests the Length and Entry functions.
func TestEntries(t *testing.T) {
	// get non-trivial test case
	tc := testCases[len(testCases)-1] // last test case
	if len(tc.entries) < 2 {
		t.Fatal("cannot test manifest entries using a test case with less than 2 entries")
	}

	m := jsonmanifest.NewManifest()
	if m.Length() != 0 {
		t.Fatalf("expected length to be %d, but is %d instead", 0, m.Length())
	}

	// add and check all entries
	for i, e := range tc.entries {
		entry := jsonmanifest.NewEntry(
			e.reference,
			e.name,
			e.headers,
		)
		path := filepath.Join(e.path, e.name)
		m.Add(path, entry)

		// verify length change
		if m.Length() != i+1 {
			t.Fatalf("expected length to be %d, but is %d instead", i+1, m.Length())
		}
		// check retrieved entry
		verifyEntry(t, m, entry, path)
	}
	// save current manifest length
	manifestLen := m.Length()

	// create new entry to replace existing one
	lastEntry := tc.entries[len(tc.entries)-1]
	entry := jsonmanifest.NewEntry(
		test.RandomAddress(),
		lastEntry.name,
		lastEntry.headers,
	)

	// replace manifest entry by adding to the same path
	path := filepath.Join(lastEntry.path, lastEntry.name)
	m.Add(path, entry)

	// length should not have changed
	if m.Length() != manifestLen {
		t.Fatalf("expected length to be %d, but is %d instead", manifestLen, m.Length())
	}
	// check retrieved entry
	verifyEntry(t, m, entry, path)

	// try removing inexistent entry
	m.Remove("invalid/path.ext")

	// length should not have changed
	if m.Length() != manifestLen {
		t.Fatalf("expected length to be %d, but is %d instead", manifestLen, m.Length())
	}

	// remove each entry
	for i, e := range tc.entries {
		path := filepath.Join(e.path, e.name)
		m.Remove(path)

		// verify length change
		if m.Length() != manifestLen-i-1 {
			t.Fatalf("expected length to be %d, but is %d instead", manifestLen-i-1, m.Length())
		}
	}
}

// verifyEntry check that an entry matches the one retrieved from the given manifest and path.
func verifyEntry(t *testing.T, m manifest.Interface, entry manifest.Entry, path string) {
	re, err := m.Entry(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entry, re) {
		t.Fatalf("original and retrieved entry are not equal: %v, %v", entry, re)
	}
}

// TestEntryModification verifies that manifest entries are read-only.
func TestEntryModification(t *testing.T) {
	m := jsonmanifest.NewManifest()

	// add single entry
	e := jsonmanifest.NewEntry(
		test.RandomAddress(),
		"single_entry.png",
		http.Header{"Content-Type": {"image/png"}},
	)
	m.Add("", e)

	// retrieve entry
	re, err := m.Entry("")
	if err != nil {
		t.Fatal(err)
	}

	// modify entry
	re.Headers().Add("Content-Type", "image/jpeg; charset=utf-8")

	// re-retrieve entry and compare
	rre, err := m.Entry("")
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(rre, re) {
		t.Fatalf("manifest entry %v was unexpectedly modified externally", rre)
	}
}

// TestMarshal verifies that created manifests are successfully marshalled and unmarshalled.
func TestMarshal(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := jsonmanifest.NewManifest()

			// add all test case entries to manifest
			for _, e := range tc.entries {
				entry := jsonmanifest.NewEntry(
					e.reference,
					e.name,
					e.headers,
				)
				m.Add(filepath.Join(e.path, e.name), entry)
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

// Struct for manifest test cases.
type testCase struct {
	name    string
	entries []e // entries to add to manifest
}

// Struct for manifest entries for test cases.
type e struct {
	reference swarm.Address
	name      string
	headers   http.Header
	path      string
}
