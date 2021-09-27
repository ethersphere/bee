package blocker

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

// var (
// 	flagTimeout    = 5 * time.Minute // how long before blocking a flagged peer
// 	cleanupTimeout = 5 * time.Minute // how long before we cleanup a peer
// 	blockDuration  = time.Hour       // how long to blocklist an unresponsive peer for
// )

type peer struct {
	flagged    bool      // indicates whether the peer is actively flagged
	blockAfter time.Time // timestamp of the point we've timed-out or got an error from a peer
	addr       swarm.Address
}

type Blocker struct {
	mux           sync.Mutex
	disconnector  p2p.Blocklister
	flagTimeout   time.Duration // how long before blocking a flagged peer
	blockDuration time.Duration // how long to blocklist a bad peer
	peers         map[string]*peer
	logger        logging.Logger
	wakeup        chan struct{}
	quit          chan struct{}
	waitingNext   bool
}

func New(dis p2p.Blocklister, flagTimeout, blockDuration time.Duration, logger logging.Logger) *Blocker {

	b := &Blocker{
		disconnector:  dis,
		flagTimeout:   flagTimeout,
		blockDuration: blockDuration,
		peers:         map[string]*peer{},
		wakeup:        make(chan struct{}),
		quit:          make(chan struct{}),
		logger:        logger,
	}

	go b.run()

	return b
}

func (b *Blocker) run() {

	for {
		select {
		case <-b.quit:
			return
		case <-b.wakeup:

			if b.waitingNext {
				continue
			}

			b.waitingNext = true
			go func() {
				<-time.After(b.flagTimeout)
				b.block()
			}()
		}
	}
}

func (b *Blocker) block() {
	b.mux.Lock()
	defer b.mux.Unlock()

	for key, peer := range b.peers {
		if time.Now().After(peer.blockAfter) {
			if err := b.disconnector.Blocklist(peer.addr, b.blockDuration); err != nil {
				b.logger.Warningf("blocker: blocking peer %s failed: %v", peer.addr, err)
			}

			delete(b.peers, key)
		}
	}

	b.waitingNext = false
}

func (b *Blocker) Flag(addr swarm.Address) {
	b.mux.Lock()
	defer b.mux.Unlock()

	p, ok := b.peers[addr.ByteString()]

	if ok {
		if !p.flagged {
			p.blockAfter = time.Now().Add(b.flagTimeout)
			p.flagged = true
		}
	} else {
		b.peers[addr.ByteString()] = &peer{
			blockAfter: time.Now().Add(b.flagTimeout),
			flagged:    true,
			addr:       addr,
		}
	}

	b.wakeup <- struct{}{}
}

func (b *Blocker) Unflag(addr swarm.Address) {
	b.mux.Lock()
	defer b.mux.Unlock()

	delete(b.peers, addr.ByteString())
}

func (b *Blocker) Close() error {
	close(b.quit)
	return nil
}
