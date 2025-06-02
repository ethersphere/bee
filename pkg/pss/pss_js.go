//go:build js
// +build js

package pss

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

type pss struct {
	key        *ecdsa.PrivateKey
	pusher     pushsync.PushSyncer
	handlers   map[Topic][]*Handler
	handlersMu sync.Mutex
	logger     log.Logger
	quit       chan struct{}
}

// New returns a new pss service.
func New(key *ecdsa.PrivateKey, logger log.Logger) Interface {
	return &pss{
		key:      key,
		logger:   logger.WithName(loggerName).Register(),
		handlers: make(map[Topic][]*Handler),
		quit:     make(chan struct{}),
	}
}

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address.
// Uses push-sync to deliver message.
func (p *pss) Send(ctx context.Context, topic Topic, payload []byte, stamper postage.Stamper, recipient *ecdsa.PublicKey, targets Targets) error {

	tc, err := Wrap(ctx, topic, payload, recipient, targets)
	if err != nil {
		return err
	}

	stamp, err := stamper.Stamp(tc.Address(), tc.Address())
	if err != nil {
		return err
	}
	tc = tc.WithStamp(stamp)

	// push the chunk using push sync so that it reaches it destination in network
	if _, err = p.pusher.PushChunkToClosest(ctx, tc); err != nil {
		if errors.Is(err, topology.ErrWantSelf) {
			return nil
		}
		return err
	}

	return nil
}
