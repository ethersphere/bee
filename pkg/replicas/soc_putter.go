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
	"github.com/ethersphere/bee/v2/pkg/replicas_soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// putter is the private implementation of the public storage.Putter interface
// putter extends the original putter to a concurrent multiputter
type socPutter struct {
	putter storage.Putter
}

// NewSocPutter is the putter constructor
func NewSocPutter(p storage.Putter) storage.Putter {
	return &socPutter{p}
}

// Put makes the getter satisfy the storage.Getter interface
func (p *socPutter) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	rlevel := redundancy.GetLevelFromContext(ctx)
	errs := []error{}
	if rlevel == 0 {
		return nil
	}

	rr := replicas_soc.NewSocReplicator(ch.Address(), rlevel)
	errc := make(chan error, rlevel.GetReplicaCount())
	wg := sync.WaitGroup{}
	for r := range rr.C {
		r := r
		wg.Add(1)
		go func() {
			defer wg.Done()
			sch := swarm.NewChunk(swarm.NewAddress(r.Addr), ch.Data())
			err = p.putter.Put(ctx, sch)
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
