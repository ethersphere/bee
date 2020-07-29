// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonmanifest

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

// JSONEntry is a JSON representation of a file entry for a JSONManifest
type JSONEntry struct {
	Reference swarm.Address `json:"reference"`
	Name      string        `json:"name"`
	Headers   http.Header   `json:"headers"`
}

// verify JSONEntry implements manifest.Entry
var _ manifest.Entry = (*JSONEntry)(nil)

// GetReference returns the address of the entry for a file
func (me JSONEntry) GetReference() swarm.Address {
	return me.Reference
}

// GetName returns the name of the file for the entry
func (me JSONEntry) GetName() string {
	return me.Name
}

// GetHeaders returns the headers for manifest entry for a file
func (me JSONEntry) GetHeaders() http.Header {
	return me.Headers
}
