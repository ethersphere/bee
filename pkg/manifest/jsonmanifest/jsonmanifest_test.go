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
				header:    http.Header{},
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
				header:    http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
				path:      "",
			},
			{
				reference: test.RandomAddress(),
				name:      "entry-2.png",
				header:    http.Header{"Content-Type": {"image/png"}},
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
				header:    http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
				path:      "text",
			},
			{
				reference: test.RandomAddress(),
				name:      "1.png",
				header:    http.Header{"Content-Type": {"image/png"}},
				path:      "img",
			},
			{
				reference: test.RandomAddress(),
				name:      "2.jpg",
				header:    http.Header{"Content-Type": {"image/jpg"}},
				path:      "img",
			},
			{
				reference: test.RandomAddress(),
				name:      "readme.md",
				header:    http.Header{"Content-Type": {"text/markdown; charset=UTF-8"}},
				path:      "",
			},
		},
	},
}

// TestEntries tests the Add, Length and Entry functions.
// This test will add multiple entries to a manifest, checking that they are correctly retrieved each time,
// and that the length of the manifest is as expected.
// It will verify that the manifest length remains unchanged when replacing entries or removing inexistent ones.
// Finally, it will remove all entries in the manifest, checking that they are correctly not found each time,
// and that the length of the manifest is as expected.
func TestEntries(t *testing.T) {
	tc := testCases[len(testCases)-1] // get non-trivial test case

	m := jsonmanifest.NewManifest()
	checkLength(t, m, 0)

	// add entries
	for i, e := range tc.entries {
		entry := jsonmanifest.NewEntry(e.reference, e.name, e.header)
		path := filepath.Join(e.path, e.name)
		m.Add(path, entry)

		checkLength(t, m, i+1)
		checkEntry(t, m, entry, path)
	}

	manifestLen := m.Length()

	// replace entry
	lastEntry := tc.entries[len(tc.entries)-1]
	path := filepath.Join(lastEntry.path, lastEntry.name)

	newEntry := jsonmanifest.NewEntry(test.RandomAddress(), lastEntry.name, lastEntry.header)
	m.Add(path, newEntry)

	checkLength(t, m, manifestLen) // length should not have changed
	checkEntry(t, m, newEntry, path)

	// remove entries
	m.Remove("invalid/path.ext")   // try removing inexistent entry
	checkLength(t, m, manifestLen) // length should not have changed

	for i, e := range tc.entries {
		path := filepath.Join(e.path, e.name)
		m.Remove(path)

		entry, err := m.Entry(path)
		if entry != nil || err != manifest.ErrNotFound {
			t.Fatalf("expected path %v not to be present in the manifest, but it was found", path)
		}

		checkLength(t, m, manifestLen-i-1)
	}
}

// checkLength verifies that the given manifest length and integer match.
func checkLength(t *testing.T, m manifest.Interface, length int) {
	if m.Length() != length {
		t.Fatalf("expected length to be %d, but is %d instead", length, m.Length())
	}
}

// checkEntry verifies that an entry is equal to the one retrieved from the given manifest and path.
func checkEntry(t *testing.T, m manifest.Interface, entry manifest.Entry, path string) {
	re, err := m.Entry(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entry, re) {
		t.Fatalf("original and retrieved entry are not equal: %v, %v", entry, re)
	}
}

// TestEntryModification verifies that manifest entries are not modifiable from outside of the manifest.
// This test will add a single entry to a manifest, retrieve it, and modify it.
// After, it will re-retrieve that same entry from the manifest, and check that it has not changed.
func TestEntryModification(t *testing.T) {
	m := jsonmanifest.NewManifest()

	e := jsonmanifest.NewEntry(test.RandomAddress(), "single_entry.png", http.Header{"Content-Type": {"image/png"}})
	m.Add("", e)

	re, err := m.Entry("")
	if err != nil {
		t.Fatal(err)
	}

	re.Header().Add("Content-Type", "text/plain; charset=utf-8") // modify retrieved entry

	rre, err := m.Entry("") // re-retrieve entry
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(rre, re) {
		t.Fatalf("manifest entry %v was unexpectedly modified externally", rre)
	}
}

// TestMarshal verifies that created manifests are successfully marshalled and unmarshalled.
// This function wil add all test case entries to a manifest and marshal it.
// After, it will unmarshal the result, and verify that it is equal to the original manifest.
func TestMarshal(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := jsonmanifest.NewManifest()

			for _, e := range tc.entries {
				entry := jsonmanifest.NewEntry(e.reference, e.name, e.header)
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

// struct for manifest test cases
type testCase struct {
	name    string
	entries []e // entries to add to manifest
}

// struct for manifest entries for test cases
type e struct {
	reference swarm.Address
	name      string
	header    http.Header
	path      string
}
