// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// the code below implements the integration of dispersed replicas in chunk upload.
// using storage.Putter interface.
package replicas

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// putter is the private implementation of the public storage.Putter interface
// putter extends the original putter to a concurrent multiputter
type putter struct {
	putter storage.Putter
}

// NewPutter is the putter constructor
func NewPutter(p storage.Putter) storage.Putter {
	return &putter{p}
}

// Put makes the getter satisfy the storage.Getter interface
func (p *putter) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	rlevel := redundancy.GetLevelFromContext(ctx)
	errs := []error{}
	if rlevel == 0 {
		return nil
	}

	rr := newReplicator(ch.Address(), rlevel)
	errc := make(chan error, rlevel.GetReplicaCount())
	wg := sync.WaitGroup{}
	for r := range rr.c {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sch, err := soc.New(r.id, ch).Sign(signer)
			if err == nil {
				err = p.putter.Put(ctx, sch)
			}
			errc <- err
		}()
	}

	wg.Wait()
	close(errc)
	for err := range errc {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
