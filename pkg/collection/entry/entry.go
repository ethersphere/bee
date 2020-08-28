// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"errors"

	"github.com/ethersphere/bee/pkg/collection"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_                           = collection.Entry(&Entry{})
	serializedDataSize          = swarm.SectionSize * 2
	encryptedSerializedDataSize = encryption.ReferenceSize * 2
)

// Entry provides addition of metadata to a data reference.
// Implements collection.Entry.
type Entry struct {
	reference swarm.Address
	metadata  swarm.Address
}

// New creates a new Entry.
func New(reference, metadata swarm.Address) *Entry {
	return &Entry{
		reference: reference,
		metadata:  metadata,
	}
}

// Reference implements collection.Entry
func (e *Entry) Reference() swarm.Address {
	return e.reference
}

// Metadata implements collection.Entry
func (e *Entry) Metadata() swarm.Address {
	return e.metadata
}

// MarshalBinary implements encoding.BinaryMarshaler
func (e *Entry) MarshalBinary() ([]byte, error) {
	br := e.reference.Bytes()
	bm := e.metadata.Bytes()
	b := append(br, bm...)
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (e *Entry) UnmarshalBinary(b []byte) error {
	var size int
	if len(b) == serializedDataSize {
		size = serializedDataSize
	} else if len(b) == encryptedSerializedDataSize {
		size = encryptedSerializedDataSize
	} else {
		return errors.New("invalid data length")
	}
	e.reference = swarm.NewAddress(b[:size/2])
	e.metadata = swarm.NewAddress(b[size/2:])
	return nil
}
