// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"encoding/json"
	"sync"

	"github.com/ethersphere/bee/pkg/manifest"
)

// verify jsonManifest implements manifest.Interface.
var _ manifest.Interface = (*jsonManifest)(nil)

// jsonManifest is a JSON representation of a manifest.
// It stores manifest entries in a map based on string keys.
type jsonManifest struct {
	entriesMu sync.RWMutex          // mutex for accessing the entries map
	Entries   map[string]*jsonEntry `json:"entries,omitempty"`
}

// NewManifest creates a new jsonManifest struct and returns a pointer to it.
func NewManifest() manifest.Interface {
	return &jsonManifest{
		Entries: make(map[string]*jsonEntry),
	}
}

// Add adds a manifest entry to the specified path.
func (m *jsonManifest) Add(path string, entry manifest.Entry) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	m.Entries[path] = &jsonEntry{
		R: entry.Reference(),
		N: entry.Name(),
		H: entry.Header(),
	}
}

// Remove removes a manifest entry on the specified path.
func (m *jsonManifest) Remove(path string) {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	delete(m.Entries, path)
}

// Entry returns a manifest entry if one is found in the specified path.
func (m *jsonManifest) Entry(path string) (manifest.Entry, error) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	entry, ok := m.Entries[path]
	if !ok {
		return nil, manifest.ErrNotFound
	}

	// return a copy to prevent external modification
	return NewEntry(entry.Reference(), entry.Name(), entry.Header().Clone()), nil
}

// Length returns the amount of entries in the manifest.
func (m *jsonManifest) Length() int {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	return len(m.Entries)
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (m *jsonManifest) MarshalBinary() ([]byte, error) {
	m.entriesMu.RLock()
	defer m.entriesMu.RUnlock()

	return json.Marshal(m)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *jsonManifest) UnmarshalBinary(b []byte) error {
	m.entriesMu.Lock()
	defer m.entriesMu.Unlock()

	return json.Unmarshal(b, m)
}
