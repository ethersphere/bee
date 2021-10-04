package reacher

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const pingTimeout = time.Second * 5

type peer struct {
	overlay swarm.Address
	addr    ma.Multiaddr
}

type reacher struct {
	mu    sync.Mutex
	queue []peer

	quit chan struct{}
	run  chan struct{}

	streamer p2p.StreamerPinger
	notifier p2p.ReachableNotifier

	wg sync.WaitGroup
}

func New(streamer p2p.StreamerPinger, notifier p2p.ReachableNotifier) *reacher {

	r := &reacher{
		quit:     make(chan struct{}),
		run:      make(chan struct{}, 1),
		streamer: streamer,
		notifier: notifier,
		wg:       sync.WaitGroup{},
	}

	r.wg.Add(1)
	go r.worker()

	return r
}

func (r *reacher) worker() {

	defer r.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case <-r.quit:
			cancel()
			return
		case <-r.run:
			r.ping(ctx)
		}
	}
}

func (r *reacher) ping(ctx context.Context) {

	r.wg.Add(1)
	defer r.wg.Done()

	for {

		select {
		case <-ctx.Done():
			return
		default:
		}

		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			return
		}
		p := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()

		ctxd, cancel := context.WithTimeout(ctx, pingTimeout)
		_, err := r.streamer.Ping(ctxd, p.addr)
		cancel()
		if err != nil {
			r.notifier.Reachable(p.overlay, false)
		} else {
			r.notifier.Reachable(p.overlay, true)
		}
	}
}

// Connected(swarm.Address, ma.Multiaddr)
// 	Disconnected(swarm.Address)
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	r.queue = append(r.queue, peer{overlay: overlay, addr: addr})
	r.mu.Unlock()

	select {
	case r.run <- struct{}{}:
	default:
	}
}

func (r *reacher) Close() error {
	close(r.quit)
	r.wg.Wait()
	return nil
}
