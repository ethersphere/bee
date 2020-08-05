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

// ErrNotFound is returned when an Entry is not found in the manifest.
var ErrNotFound = errors.New("manifest: not found")

// Interface for operations with manifest.
type Interface interface {
	// Add a manifest entry to the specified path.
	Add(string, Entry)
	// Remove a manifest entry on the specified path.
	Remove(string)
	// Entry returns a manifest entry if one is found in the specified path.
	Entry(string) (Entry, error)
	// Length returns an implementation-specific count of elements in the manifest.
	Length() int
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// Entry represents a single manifest entry.
type Entry interface {
	// Reference returns the address of the file in the entry.
	Reference() swarm.Address
	// Name returns the name of the file in the entry.
	Name() string
	// Header returns the HTTP header for the file in the manifest entry.
	Header() http.Header
}
