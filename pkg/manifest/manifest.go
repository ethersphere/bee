// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"encoding"
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/swarm"
)

// ErrNotFound is returned when an Entry is not found in the manifest
var ErrNotFound = errors.New("manifest: not found")

// Interface for operations with manifest
type Interface interface {
	// Add a manifest entry to the specified path
	Add(string, Entry)
	// Remove a manifest entry on the specified path
	Remove(string)
	// FindEntry returns a manifest entry if one is found in the specified path
	FindEntry(string) (Entry, error)
	// BinaryMarshaler is the interface implemented by an object that can marshal itself into a binary form
	encoding.BinaryMarshaler
	// BinaryUnmarshaler is the interface implemented by an object that can unmarshal a binary representation of itself
	encoding.BinaryUnmarshaler
}

// Entry represents a single manifest entry
type Entry interface {
	// GetReference returns the address of the file in the entry
	GetReference() swarm.Address
	// GetName returns the name of the file in the entry
	GetName() string
	// GetHeaders returns the headers for the file in the manifest entry
	GetHeaders() http.Header
}
