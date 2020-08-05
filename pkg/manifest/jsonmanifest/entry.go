// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

// verify jsonEntry implements manifest.Entry.
var _ manifest.Entry = (*jsonEntry)(nil)

// jsonEntry is a JSON representation of a single manifest entry for a jsonManifest.
type jsonEntry struct {
	R swarm.Address `json:"reference"`
	N string        `json:"name"`
	H http.Header   `json:"header"`
}

// NewEntry creates a new jsonEntry struct and returns it.
func NewEntry(reference swarm.Address, name string, headers http.Header) manifest.Entry {
	return &jsonEntry{
		R: reference,
		N: name,
		H: headers,
	}
}

// Reference returns the address of the file in the entry.
func (me *jsonEntry) Reference() swarm.Address {
	return me.R
}

// Name returns the name of the file in the entry.
func (me *jsonEntry) Name() string {
	return me.N
}

// Header returns the HTTP header for the file in the manifest entry.
func (me *jsonEntry) Header() http.Header {
	return me.H
}
