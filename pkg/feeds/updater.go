package feeds

import (
	"context"
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Updater encapsulates a chunk store putter and a Feed to generate successive updates
type Updater struct {
	storage.Putter
	signer crypto.Signer
	*Feed
	last  int64
	epoch *epoch
}

// NewUpdater constructs a feed updater
func NewUpdater(putter storage.Putter, signer crypto.Signer, topic string) (*Updater, error) {
	owner, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}
	feed, err := New(topic, owner)
	if err != nil {
		return nil, err
	}
	return &Updater{putter, signer, feed, 0, nil}, nil
}

// Update pushes an update to the feed through the chunk stores
func (u *Updater) Update(ctx context.Context, at int64, payload []byte) error {
	e := next(u.epoch, u.last, uint64(at))
	fu := &update{u.Feed, e}
	id, err := fu.id()
	if err != nil {
		return err
	}
	cac, err := toChunk(uint64(at), payload)
	if err != nil {
		return err
	}
	ch, err := soc.NewChunk(id, cac, u.signer)
	if err != nil {
		return err
	}

	_, err = u.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		return err
	}
	u.last = at
	u.epoch = e
	return nil
}

func toChunk(at uint64, payload []byte) (swarm.Chunk, error) {
	hasher := bmtpool.Get()
	defer hasher.Reset()
	defer bmtpool.Put(hasher)

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, at)
	content := append(ts, payload...)
	_, err := hasher.Write(content)
	if err != nil {
		return nil, err
	}
	span := make([]byte, 8)
	binary.BigEndian.PutUint64(span, uint64(len(content)))
	err = hasher.SetSpanBytes(span)
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(swarm.NewAddress(hasher.Sum(nil)), append(append([]byte{}, span...), content...)), nil
}
