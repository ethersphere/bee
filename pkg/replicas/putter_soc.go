// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// the code below implements the integration of dispersed replicas in SOC upload.
// using storer.PutterSession interface.
package replicas

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// maxRedundancyLevel ensures that no more than 2^4 = 16 replicas are generated
const maxRedundancyLevel = 4

// socPutter is the implementation of the public storage.Putter interface.
// socPutter extends the original putter to a concurrent multiputter.
type socPutter struct {
	putter storage.Putter
	level  redundancy.Level
}

// NewSocPutter is the putter constructor.
func NewSocPutter(p storage.Putter, level redundancy.Level) storage.Putter {
	return &socPutter{
		putter: p,
		level:  min(level, maxRedundancyLevel),
	}
}

// Put makes the putter satisfy the storage.Putter interface.
func (p *socPutter) Put(ctx context.Context, ch swarm.Chunk) error {
	if err := p.putter.Put(ctx, ch); err != nil {
		return fmt.Errorf("put original chunk: %w", err)
	}

	var errs error

	for replicaAddr := range combinator.IterateReplicaAddresses(ch.Address(), int(p.level)) {
		ch := swarm.NewChunk(replicaAddr, ch.Data())

		if err := p.putter.Put(ctx, ch); err != nil {
			errs = errors.Join(errs, fmt.Errorf("put replica chunk %v: %w", ch.Address(), err))
		}
	}

	return errs
}

// socPutterSession extends the original socPutter.
type socPutterSession struct {
	socPutter
	ps storer.PutterSession
}

// NewSocPutterSession is the putterSession constructor.
func NewSocPutterSession(p storer.PutterSession, rLevel redundancy.Level) storer.PutterSession {
	return &socPutterSession{
		socPutter{
			putter: p,
			level:  rLevel,
		}, p,
	}
}

func (p *socPutterSession) Cleanup() error {
	return p.ps.Cleanup()
}

func (p *socPutterSession) Done(addr swarm.Address) error {
	return p.ps.Done(addr)
}
