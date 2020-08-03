// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"encoding/json"

	"github.com/ethersphere/bee/pkg/manifest"
)

// verify JSONManifest implements manifest.Interface.
var _ manifest.Interface = (*JSONManifest)(nil)

// JSONManifest is a JSON representation of a manifest.
// It stores manifest entries in a map based on string keys.
type JSONManifest struct {
	Entries map[string]*JSONEntry `json:"entries,omitempty"`
}

// NewManifest creates a new JSONManifest struct and returns a pointer to it.
func NewManifest() *JSONManifest {
	return &JSONManifest{
		Entries: make(map[string]*JSONEntry),
	}
}

// Add adds a manifest entry to the specified path.
func (m *JSONManifest) Add(path string, entry manifest.Entry) {
	m.Entries[path] = NewEntry(entry.Reference(), entry.Name(), entry.Headers())
}

// Remove removes a manifest entry on the specified path.
func (m *JSONManifest) Remove(path string) {
	delete(m.Entries, path)
}

// Entry returns a manifest entry if one is found in the specified path.
func (m *JSONManifest) Entry(path string) (manifest.Entry, error) {
	if entry, ok := m.Entries[path]; ok {
		return entry, nil
	}

	return nil, manifest.ErrNotFound
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (m *JSONManifest) MarshalBinary() (data []byte, err error) {
	return json.Marshal(m)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (m *JSONManifest) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}
