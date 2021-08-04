package stampputter

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type stamperPutter struct {
	storage.Storer
	stamper postage.Stamper
}

func New(s storage.Storer, post postage.Service, signer crypto.Signer, batch []byte) (storage.Storer, error) {
	i, err := post.GetStampIssuer(batch)
	if err != nil {
		return nil, fmt.Errorf("stamp issuer: %w", err)
	}

	stamper := postage.NewStamper(i, signer)
	return &stamperPutter{Storer: s, stamper: stamper}, nil
}

func (p *stamperPutter) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exists []bool, err error) {
	var (
		ctp []swarm.Chunk
		idx []int
	)
	exists = make([]bool, len(chs))

	for i, c := range chs {
		has, err := p.Storer.Has(ctx, c.Address())
		if err != nil {
			return nil, err
		}
		if has || containsChunk(c.Address(), chs[:i]...) {
			exists[i] = true
			continue
		}
		stamp, err := p.stamper.Stamp(c.Address())
		if err != nil {
			return nil, err
		}
		chs[i] = c.WithStamp(stamp)
		ctp = append(ctp, chs[i])
		idx = append(idx, i)
	}

	exists2, err := p.Storer.Put(ctx, mode, ctp...)
	if err != nil {
		return nil, err
	}
	for i, v := range idx {
		exists[v] = exists2[i]
	}
	return exists, nil
}

// containsChunk returns true if the chunk with a specific address
// is present in the provided chunk slice.
func containsChunk(addr swarm.Address, chs ...swarm.Chunk) bool {
	for _, c := range chs {
		if addr.Equal(c.Address()) {
			return true
		}
	}
	return false
}
