// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"encoding/json"
	"sync"

	"github.com/ethersphere/bee/pkg/manifest"
)

// verify JSONManifest implements manifest.Interface.
var _ manifest.Interface = (*jsonManifest)(nil)

// jsonManifest is a JSON representation of a manifest.
// It stores manifest entries in a map based on string keys.
type jsonManifest struct {
	entriesMu sync.RWMutex // mutex for accessing the entries map
	entries   map[string]*JSONEntry
}

// NewManifest creates a new JSONManifest struct and returns a pointer to it.
func NewManifest() manifest.Interface {
	return &jsonManifest{
		entries: make(map[string]*JSONEntry),
	}
}

// Add adds a manifest entry to the specified path.
func (m *jsonManifest) Add(path string, entry manifest.Entry) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	m.entries[path] = NewEntry(entry.Reference(), entry.Name(), entry.Headers())
}

// Remove removes a manifest entry on the specified path.
func (m *jsonManifest) Remove(path string) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	delete(m.entries, path)
}

// Entry returns a manifest entry if one is found in the specified path
func (m *jsonManifest) Entry(path string) (manifest.Entry, error) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	if entry, ok := m.entries[path]; ok {
		// return a copy to prevent external modification
		return NewEntry(entry.Reference(), entry.Name(), entry.Headers()), nil
	}

	return nil, manifest.ErrNotFound
}

// Size returns the amount of entries in the manifest.
func (m *jsonManifest) Size() int {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	return len(m.entries)
}

// exportManifest is a struct used for marshaling and unmarshaling JSONManifest structs.
type exportManifest struct {
	Entries map[string]*JSONEntry `json:"entries,omitempty"`
}

// MarshalBinary implements encoding.BinaryMarshaler
func (m *jsonManifest) MarshalBinary() ([]byte, error) {
	return json.Marshal(exportManifest{
		Entries: m.entries,
	})
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *jsonManifest) UnmarshalBinary(b []byte) error {
	e := exportManifest{}
	if err := json.Unmarshal(b, &e); err != nil {
		return err
	}
	m.entries = e.Entries
	return nil
}
