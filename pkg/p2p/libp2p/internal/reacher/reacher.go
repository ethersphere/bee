//go:build !js
// +build !js

package reacher

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
)

type reacher struct {
	mu    sync.Mutex
	peers map[string]*peer

	newPeer chan struct{}
	quit    chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg      sync.WaitGroup
	metrics metrics

	options *Options
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier, o *Options) *reacher {

	r := &reacher{
		newPeer:  make(chan struct{}, 1),
		quit:     make(chan struct{}),
		pinger:   streamer,
		peers:    make(map[string]*peer),
		notifier: notifier,
		metrics:  newMetrics(),
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

		now := time.Now()

		ctxt, cancel := context.WithTimeout(ctx, r.options.PingTimeout)
		_, err := r.pinger.Ping(ctxt, p.addr)
		cancel()

		// ping was successful
		if err == nil {
			r.metrics.Pings.WithLabelValues("success").Inc()
			r.metrics.PingTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
		} else {
			r.metrics.Pings.WithLabelValues("failure").Inc()
			r.metrics.PingTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())
			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
		}
	}
}
