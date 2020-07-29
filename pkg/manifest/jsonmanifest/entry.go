// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

// verify JSONEntry implements manifest.Entry
var _ manifest.Entry = (*JSONEntry)(nil)

// JSONEntry is a JSON representation of a single manifest entry for a JSONManifest
type JSONEntry struct {
	Reference swarm.Address `json:"reference"`
	Name      string        `json:"name"`
	Headers   http.Header   `json:"headers"`
}

// GetReference returns the address of the file in the entry
func (me JSONEntry) GetReference() swarm.Address {
	return me.Reference
}

// GetName returns the name of the file in the entry
func (me JSONEntry) GetName() string {
	return me.Name
}

// GetHeaders returns the headers for the file in the manifest entry
func (me JSONEntry) GetHeaders() http.Header {
	return me.Headers
}
