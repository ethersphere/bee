//go:build js
// +build js

package reacher

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/p2p"
)

type reacher struct {
	mu    sync.Mutex
	peers map[string]*peer

	newPeer chan struct{}
	quit    chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg sync.WaitGroup

	options *Options
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier, o *Options) *reacher {

	r := &reacher{
		newPeer:  make(chan struct{}, 1),
		quit:     make(chan struct{}),
		pinger:   streamer,
		peers:    make(map[string]*peer),
		notifier: notifier,
	}

	if o == nil {
		o = &Options{
			PingTimeout:        pingTimeout,
			Workers:            workers,
			RetryAfterDuration: retryAfterDuration,
		}
	}
	r.options = o

	r.wg.Add(1)
	go r.manage()

	return r
}

func (r *reacher) ping(c chan *peer, ctx context.Context) {

	defer r.wg.Done()

	for p := range c {

		ctxt, cancel := context.WithTimeout(ctx, r.options.PingTimeout)
		_, err := r.pinger.Ping(ctxt, p.addr)
		cancel()

		// ping was successful
		if err == nil {

			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
		} else {

			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
		}
	}
}
