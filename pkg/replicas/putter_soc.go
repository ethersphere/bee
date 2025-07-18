// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// the code below implements the integration of dispersed replicas in SOC upload.
// using storer.PutterSession interface.
package replicas

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// socPutter is the private implementation of the public storage.Putter interface
// socPutter extends the original putter to a concurrent multiputter
type socPutter struct {
	putter storage.Putter
	rLevel redundancy.Level
}

// NewSocPutter is the putter constructor
func NewSocPutter(p storage.Putter, rLevel redundancy.Level) storage.Putter {
	return &socPutter{
		putter: p,
		rLevel: rLevel,
	}
}

// Put makes the putter satisfy the storage.Putter interface
func (p *socPutter) Put(ctx context.Context, ch swarm.Chunk) error {
	errs := []error{}
	// Put base chunk first
	if err := p.putter.Put(ctx, ch); err != nil {
		return err
	}
	if p.rLevel == 0 {
		return nil
	}

	rr := newSocReplicator(ch.Address(), p.rLevel)
	errc := make(chan error, p.rLevel.GetReplicaCount())
	wg := sync.WaitGroup{}
	for r := range rr.c {
		wg.Add(1)
		go func(r *socReplica) {
			defer wg.Done()
			// create a new chunk with the replica address
			sch := swarm.NewChunk(swarm.NewAddress(r.addr), ch.Data())
			err := p.putter.Put(ctx, sch)
			errc <- err
		}(r)
	}

	wg.Wait()
	close(errc)
	for err := range errc {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// socPutterSession extends the original socPutter
type socPutterSession struct {
	socPutter
	ps storer.PutterSession
}

// NewSocPutter is the putterSession constructor
func NewSocPutterSession(p storer.PutterSession, rLevel redundancy.Level) storer.PutterSession {
	return &socPutterSession{
		socPutter{
			putter: p,
			rLevel: rLevel,
		}, p,
	}
}

func (p *socPutterSession) Cleanup() error {
	return p.ps.Cleanup()
}

func (p *socPutterSession) Done(addr swarm.Address) error {
	return p.ps.Done(addr)
}
