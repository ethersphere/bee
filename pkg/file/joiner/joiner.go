// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implemenations of the file.Joiner interface
package joiner

import (
	"io"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type simpleJoiner struct {
	store storage.Storer
	logger logging.Logger
}

func (s *simpleJoiner) Join(address swarm.Address, dataReader io.Reader) (dataSize int64, err error) {
	s.logger.Warning("Join method is noop")
	return 0, nil
}

func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store: store,
	}
}
