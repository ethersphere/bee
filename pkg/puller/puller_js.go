//go:build js
// +build js

package puller

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/rate"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	ratelimit "golang.org/x/time/rate"
)

type Puller struct {
	base swarm.Address

	topology    topology.Driver
	radius      storer.RadiusChecker
	statestore  storage.StateStorer
	syncer      pullsync.Interface
	blockLister p2p.Blocklister

	logger log.Logger

	syncPeers    map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex
	intervalMtx  sync.Mutex

	cancel func()

	wg sync.WaitGroup

	bins uint8 // how many bins do we support

	rate *rate.Rate // rate of historical syncing

	start sync.Once

	limiter *ratelimit.Limiter
}

func New(
	addr swarm.Address,
	stateStore storage.StateStorer,
	topology topology.Driver,
	reserveState storer.RadiusChecker,
	pullSync pullsync.Interface,
	blockLister p2p.Blocklister,
	logger log.Logger,
	o Options,
) *Puller {
	bins := swarm.MaxBins
	if o.Bins != 0 {
		bins = o.Bins
	}
	p := &Puller{
		base:        addr,
		statestore:  stateStore,
		topology:    topology,
		radius:      reserveState,
		syncer:      pullSync,
		logger:      logger.WithName(loggerName).Register(),
		syncPeers:   make(map[string]*syncPeer),
		bins:        bins,
		blockLister: blockLister,
		rate:        rate.New(DefaultHistRateWindow),
		cancel:      func() { /* Noop, since the context is initialized in the Start(). */ },
		limiter:     ratelimit.NewLimiter(ratelimit.Every(time.Second/maxChunksPerSecond), maxChunksPerSecond),
	}

	return p
}

// syncPeerBin will start historical and live syncing for the peer for a particular bin.
// Must be called under syncPeer lock.
func (p *Puller) syncPeerBin(parentCtx context.Context, peer *syncPeer, bin uint8, cursor uint64) {
	loggerV2 := p.logger.V(2).Register()

	ctx, cancel := context.WithCancel(parentCtx)
	peer.setBinCancel(cancel, bin)

	sync := func(isHistorical bool, address swarm.Address, start uint64) {
		defer p.wg.Done()
		defer peer.wg.Done()

		var err error

		for {
			if isHistorical { // override start with the next interval if historical syncing
				start, err = p.nextPeerInterval(address, bin)
				if err != nil {
					p.logger.Error(err, "syncWorker nextPeerInterval failed, quitting")
					return
				}

				// historical sync has caught up to the cursor, exit
				if start > cursor {
					return
				}
			}

			select {
			case <-ctx.Done():
				loggerV2.Debug("syncWorker context cancelled", "peer_address", address, "bin", bin)
				return
			default:
			}

			syncStart := time.Now()
			top, count, err := p.syncer.Sync(ctx, address, bin, start)

			if top == math.MaxUint64 {
				p.logger.Error(nil, "syncWorker max uint64 encountered, quitting", "peer_address", address, "bin", bin, "from", start, "topmost", top)
				return
			}

			if err != nil {
				if errors.Is(err, p2p.ErrPeerNotFound) {
					p.logger.Debug("syncWorker interval failed, quitting", "error", err, "peer_address", address, "bin", bin, "cursor", cursor, "start", start, "topmost", top)
					return
				}
				loggerV2.Debug("syncWorker interval failed", "error", err, "peer_address", address, "bin", bin, "cursor", cursor, "start", start, "topmost", top)
			}

			_ = p.limiter.WaitN(ctx, count)

			if isHistorical {
				p.rate.Add(count)
			}

			// pulled at least one chunk
			if top >= start {
				if err := p.addPeerInterval(address, bin, start, top); err != nil {
					p.logger.Error(err, "syncWorker could not persist interval for peer, quitting", "peer_address", address)
					return
				}
				loggerV2.Debug("syncWorker pulled", "bin", bin, "start", start, "topmost", top, "isHistorical", isHistorical, "duration", time.Since(syncStart), "peer_address", address)
				start = top + 1
			}
		}
	}

	if cursor > 0 {
		peer.wg.Add(1)
		p.wg.Add(1)
		go sync(true, peer.address, cursor)
	}

	peer.wg.Add(1)
	p.wg.Add(1)
	go sync(false, peer.address, cursor+1)
}
