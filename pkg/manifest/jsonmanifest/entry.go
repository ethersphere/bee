// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

// verify JSONEntry implements manifest.Entry.
var _ manifest.Entry = (*JSONEntry)(nil)

// JSONEntry is a JSON representation of a single manifest entry for a JSONManifest.
type JSONEntry struct {
	reference swarm.Address
	name      string
	headers   http.Header
}

// NewEntry creates a new JSONEntry struct and returns it.
func NewEntry(reference swarm.Address, name string, headers http.Header) JSONEntry {
	return JSONEntry{
		reference: reference,
		name:      name,
		headers:   headers,
	}
}

// Reference returns the address of the file in the entry.
func (me JSONEntry) Reference() swarm.Address {
	return me.reference
}

// Name returns the name of the file in the entry.
func (me JSONEntry) Name() string {
	return me.name
}

// Headers returns the headers for the file in the manifest entry.
func (me JSONEntry) Headers() http.Header {
	return me.headers
}
