// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implemenations of the file.Joiner interface
package joiner

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type simpleJoiner struct {
	store storage.Storer
	logger logging.Logger
}

func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.Reader, dataSize int64, err error) {
	s.logger.Warning("foo")
	ch, err := s.store.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return nil, 0, err
	}
	r := bytes.NewReader(ch.Data())
	return r, int64(len(ch.Data())), nil
}

func NewSimpleJoiner(store storage.Storer) file.Joiner {
	return &simpleJoiner{
		store: store,
		logger: logging.New(ioutil.Discard, 0),

	}
}
