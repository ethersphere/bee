// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"encoding/json"

	"github.com/ethersphere/bee/pkg/manifest"
)

var _ manifest.Parser = (*MockManifestParser)(nil)

type MockManifestParser struct {
}

func NewManifestParser() *MockManifestParser {
	mp := &MockManifestParser{}

	return mp
}

func (m *MockManifestParser) Parse(bytes []byte) (manifest.Interface, error) {
	mi := &MockManifestInterface{}
	err := json.Unmarshal(bytes, mi)

	return mi, err
}

var _ manifest.Interface = (*MockManifestInterface)(nil)

type MockManifestInterface struct {
	Entries map[string]manifest.Entry `json:"entries,omitempty"`
}

func NewManifestInterface(entries map[string]manifest.Entry) *MockManifestInterface {
	mi := &MockManifestInterface{
		Entries: entries,
	}

	return mi
}

func (m *MockManifestInterface) FindEntry(path string) (*manifest.Entry, error) {
	if entry, ok := m.Entries[path]; ok {
		return &entry, nil
	}

	return nil, manifest.ErrNotFound
}

func (m *MockManifestInterface) Serialize() []byte {
	manifestBytes, _ := json.Marshal(m)

	return manifestBytes
}
