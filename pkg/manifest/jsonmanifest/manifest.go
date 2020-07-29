// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"encoding/json"

	"github.com/ethersphere/bee/pkg/manifest"
)

// JSONManifest stores JSON manifest entries
type JSONManifest struct {
	Entries map[string]JSONEntry `json:"entries,omitempty"`
}

// verify JSONManifest implements manifest.Interface
var _ manifest.Interface = (*JSONManifest)(nil)

// NewManifest creates a new JSONManifest struct and returns a pointer to it
func NewManifest() *JSONManifest {
	return &JSONManifest{
		Entries: make(map[string]JSONEntry),
	}
}

// Add adds a manifest entry to the specified path
func (m *JSONManifest) Add(path string, entry manifest.Entry) {
	m.Entries[path] = JSONEntry{
		Reference: entry.GetReference(),
		Name:      entry.GetName(),
		Headers:   entry.GetHeaders(),
	}
}

// Remove removes a manifest entry on the specified path
func (m *JSONManifest) Remove(path string) {
	delete(m.Entries, path)
}

// FindEntry returns a manifest entry if one is found on the specified path
func (m *JSONManifest) FindEntry(path string) (manifest.Entry, error) {
	if entry, ok := m.Entries[path]; ok {
		return entry, nil
	}

	return nil, manifest.ErrNotFound
}

// MarshalBinary implements encoding.BinaryMarshaler
func (m *JSONManifest) MarshalBinary() (data []byte, err error) {
	return json.Marshal(m)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (m *JSONManifest) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}
