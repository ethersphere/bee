// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"errors"

	"github.com/ethersphere/bee/pkg/collection"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	serializedDataSize = swarm.SectionSize * 2
)

type Entry struct {
	reference swarm.Address
	metadata      swarm.Address
}

func New(reference swarm.Address) *Entry {
	return &Entry{
		reference: reference,
	}
}

func (e *Entry) SetMetadata(metadataAddress swarm.Address) {
	e.metadata = metadataAddress
}

func (e *Entry) Reference() swarm.Address {
	return e.reference
}

func (e *Entry) Metadata(collection.MetadataType) swarm.Address {
	return e.metadata
}

func (e *Entry) MarshalBinary() ([]byte, error) {
	br := e.reference.Bytes()
	bm := e.metadata.Bytes()
	b := append(br, bm...)
	return b, nil
}

func (e *Entry) UnmarshalBinary(b []byte) error {
	if len(b) != serializedDataSize  {
		return errors.New("Invalid data length")
	}
	e.reference = swarm.NewAddress(b[:swarm.SectionSize])
	e.metadata = swarm.NewAddress(b[swarm.SectionSize:])
	return nil
}
