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
var _ manifest.Interface = (*JSONManifest)(nil)

// JSONManifest is a JSON representation of a manifest.
// It stores manifest entries in a map based on string keys.
type JSONManifest struct {
	entriesMu sync.RWMutex // mutex for accessing the entries map
	entries   map[string]*JSONEntry
}

// NewManifest creates a new JSONManifest struct and returns a pointer to it.
func NewManifest() *JSONManifest {
	return &JSONManifest{
		entries: make(map[string]*JSONEntry),
	}
}

// Add adds a manifest entry to the specified path.
func (m *JSONManifest) Add(path string, entry manifest.Entry) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	m.entries[path] = NewEntry(entry.Reference(), entry.Name(), entry.Headers())
}

// Remove removes a manifest entry on the specified path.
func (m *JSONManifest) Remove(path string) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	delete(m.entries, path)
}

// Entry returns a manifest entry if one is found in the specified path
func (m *JSONManifest) Entry(path string) (manifest.Entry, error) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	if entry, ok := m.entries[path]; ok {
		return entry, nil
	}

	return nil, manifest.ErrNotFound
}

// Entries returns a copy of the JSONManifest entries field
func (m *JSONManifest) Entries() map[string]*JSONEntry {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	return m.entries
}

// exportEntry is a struct used for marshaling and unmarshaling JSONEntry structs.
type exportManifest struct {
	Entries map[string]*JSONEntry `json:"entries,omitempty"`
}

// MarshalBinary implements encoding.BinaryMarshaler
func (m *JSONManifest) MarshalBinary() ([]byte, error) {
	return json.Marshal(exportManifest{
		Entries: m.entries,
	})
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *JSONManifest) UnmarshalBinary(b []byte) error {
	e := exportManifest{}
	if err := json.Unmarshal(b, &e); err != nil {
		return err
	}
	m.entries = e.Entries
	return nil
}
