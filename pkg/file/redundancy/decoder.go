// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type decoder struct {

}

type getter struct {
	storage.Getter
	decoders map[string] *decoder // only for effective chunks
	//TODO fields
	// strategy constant enum 3 types 
}

func NewGetter(g storage.Getter) storage.Getter {
	return &getter{Getter: g}
}

// Get will not call parities
func (g *getter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	ch, err := g.Getter.Get(ctx, addr)

	switch {
		case err == nil:
			// if IsParityEncoded(ch.Data()[:swarm.SpanSize]) {
				parities := DecodeParity(ch.Data()[:swarm.SpanSize])
				// decode span -> span and parity
				if parities > 0 {
					decoder := newDecoder(ch.Data())
					// -> decoder.addresses
					// decoders map 
					sctx, cancel := context.WithCancel(ctx)
					// create a wait function that awaits for the sctx to be done and we need channel
					wait := newWaiter(sctx, g, stopChannel) 

					// create subContext
					// iterate over real effective shard addresses and insert all addresses into the map with wait as value.
					// start strategy on `decoder` with passing the context
					// api create getter pass strategy and just passing context
					sync.Once
					go g.launchStrategy(ctx, decoder) {
						// if successful save parities (later)

					}
				}
				return ch, nil
			// }

		case !errors.Is(storage.ErrNotFound, err):
			return nil, err
	}
}
