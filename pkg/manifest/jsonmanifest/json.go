// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"encoding/json"
	"net/http"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ manifest.Parser = (*JSONParser)(nil)

type JSONParser struct{}

func NewParser() *JSONParser {
	return &JSONParser{}
}

func (m *JSONParser) Parse(bytes []byte) (manifest.Interface, error) {
	mi := &JSONManifest{}
	err := json.Unmarshal(bytes, mi)

	return mi, err
}

var _ manifest.Interface = (*JSONManifest)(nil)

type JSONManifest struct {
	Entries map[string]JSONEntry `json:"entries,omitempty"`
}

func NewManifest() *JSONManifest {
	return &JSONManifest{
		Entries: make(map[string]JSONEntry),
	}
}

func (m *JSONManifest) Add(path string, entry manifest.Entry) {
	m.Entries[path] = JSONEntry{
		Reference: entry.GetReference(),
		Name:      entry.GetName(),
		Headers:   entry.GetHeaders(),
	}
}

func (m *JSONManifest) Remove(path string) {
	delete(m.Entries, path)
}

func (m *JSONManifest) FindEntry(path string) (manifest.Entry, error) {
	if entry, ok := m.Entries[path]; ok {
		return entry, nil
	}

	return nil, manifest.ErrNotFound
}

func (m *JSONManifest) Serialize() ([]byte, error) {
	return json.Marshal(m)
}

var _ manifest.Entry = (*JSONEntry)(nil)

type JSONEntry struct {
	Reference swarm.Address `json:"reference"`
	Name      string        `json:"name"`
	Headers   http.Header   `json:"headers"`
}

func (me JSONEntry) GetReference() swarm.Address {
	return me.Reference
}

func (me JSONEntry) GetName() string {
	return me.Name
}

func (me JSONEntry) GetHeaders() http.Header {
	return me.Headers
}
