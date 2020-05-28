// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"github.com/ethersphere/bee/pkg/collection"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Entry struct {
	reference swarm.Address
	meta      swarm.Address
}

func New(reference swarm.Address) collection.Entry {
	return &Entry{
		reference: reference,
	}
}

func (e *Entry) Reference() swarm.Address {
	return e.reference
}

func (e *Entry) Metadata(collection.MetadataType) swarm.Address {
	return swarm.ZeroAddress
}
