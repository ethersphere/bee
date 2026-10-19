// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "rangesync"

// Protocol and stream names (proposal §6.1).
const (
	ProtocolName    = "rangesync"
	ProtocolVersion = "1.0.0"

	// StatusStreamName is the stream on which peers gossip their root summary.
	StatusStreamName = "status"
	// ReconcileStreamName is the stream of one reconciliation session, opened
	// by the initiator.
	ReconcileStreamName = "reconcile"
	// FetchStreamName is the stream on which a requester asks for chunks by ID
	// and the holder delivers them.
	FetchStreamName = "fetch"
)

var (
	// ErrNoStatus is returned by Reconcile when no Status of the peer was
	// received yet.
	ErrNoStatus = errors.New("rangesync: no status from peer")
	// ErrMappingMismatch is returned by Reconcile when the peer's Status has a
	// Mapping that differs from this node's: the peer is not eligible for
	// sessions. The reconcile handler resets the stream with it.
	ErrMappingMismatch = errors.New("rangesync: mapping mismatch")
	// ErrSessionLimit is returned by Reconcile, without opening a stream,
	// when this node already runs MaxConcurrentSessions sessions or one
	// with the peer (incoming or outgoing).
	ErrSessionLimit = errors.New("rangesync: session limit")
	// ErrNodeSessionLimit is the ErrSessionLimit of a node that already runs
	// MaxConcurrentSessions sessions, a cause on this node.
	ErrNodeSessionLimit = fmt.Errorf("%w: node", ErrSessionLimit)
	// ErrPeerSessionLimit is the ErrSessionLimit of a node that already runs
	// a session with the peer, incoming or outgoing.
	ErrPeerSessionLimit = fmt.Errorf("%w: peer", ErrSessionLimit)
	// ErrProtocolUnsupported is returned (wrapped) by SendStatus when the
	// peer does not support the rangesync protocol (the stream could not be
	// negotiated, p2p.IncompatibleStreamError). The reconciler does not
	// probe such a peer again while it stays connected.
	ErrProtocolUnsupported = errors.New("rangesync: protocol not supported by peer")
)

// PeerScorer receives the outcome of the sessions with each peer. The
// reconciler implements it and keeps the single per-peer failure score.
type PeerScorer interface {
	// Fail reports that peer broke the protocol; reason wraps
	// ErrInvalidMessage. The peer was disconnected.
	Fail(peer swarm.Address, reason error)
	// Succeed reports that a session with peer completed.
	Succeed(peer swarm.Address)
}

// noopScorer is the default PeerScorer.
type noopScorer struct{}

func (noopScorer) Fail(swarm.Address, error) {}
func (noopScorer) Succeed(swarm.Address)     {}

// Store is the part of the storer that the rangesync service uses: the
// address index (storer.ReserveAddrStore, proposal §5.4), the reserve radius
// and the reserve putter. *storer.DB implements it.
type Store interface {
	IndexerStore
	// ReserveHasID reports whether (addr, stampHash) is in the reserve.
	ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error)
	// ReserveGetByID returns the chunk of (addr, stampHash) in the reserve,
	// or an error wrapping storage.ErrNotFound.
	ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error)
	// ReservePutter returns the putter that stores fetched chunks.
	ReservePutter() storage.Putter
	// IsWithinStorageRadius reports whether addr is in the neighborhood.
	IsWithinStorageRadius(addr swarm.Address) bool
	// SetReserveObserver registers the single observer of the reserve.
	SetReserveObserver(o storer.ReserveObserver) error
}

var _ Store = (*storer.DB)(nil)

// Options configures a Service.
type Options struct {
	// Scorer receives the outcome of sessions. Nil means no scoring.
	Scorer PeerScorer
	// Wants, if not nil, receives the IDs that the peer of a session this
	// node served listed and this node does not hold, once the session
	// completed. The batch IDs are not known and are nil.
	Wants func(peer swarm.Address, wants []storer.ChunkID)
	// LeafBits is the leaf depth of the fingerprint index below its root.
	// Zero means fingerprint.LeafBits; other values are for tests.
	LeafBits uint8
	// Indexer configures the Indexer that keeps the index current.
	Indexer IndexerOptions
	// FetchRate is the node-wide limit of chunks requested per second by
	// Fetch. Zero means DefaultFetchRate.
	FetchRate int
	// FetchTimeout bounds one Want, from opening its stream to the last
	// Delivery. Zero means ClaimDeadline / 2, so that a stalled holder
	// releases the claims before they expire.
	FetchTimeout time.Duration
}

var _ io.Closer = (*Service)(nil)

// Service is the rangesync protocol service. It serves the status, reconcile
// and fetch streams and opens them towards peers. It owns the fingerprint
// index of the node's neighborhood and the Indexer that keeps it current with
// the reserve.
type Service struct {
	streamer p2p.StreamerDisconnecter
	store    Store
	logger   log.Logger
	metrics  metrics
	scorer   PeerScorer
	wants    func(peer swarm.Address, wants []storer.ChunkID)

	unwrap      func(swarm.Chunk)
	gsocHandler func(*soc.SOC)
	validStamp  postage.ValidStampFn

	fetchTimeout time.Duration

	wantSet         *wantset.Set
	fetchLimiter    *ratelimit.Limiter // node-wide, requester side
	deliveryLimiter *ratelimit.Limiter // per peer, holder side
	sessionLimiter  *ratelimit.Limiter // per peer, sessions served
	statusLimiter   *ratelimit.Limiter // per peer, statuses received

	index   *fingerprint.Index
	indexer *Indexer
	local   *local

	hints *hintCache // hint(R) of the symbol estimate

	statusMu   sync.Mutex
	statuses   map[string]Status // last Status received, by peer
	statusSubs []chan struct{}   // SubscribeStatus channels

	statusStreams    *peerStreamLimiter
	reconcileStreams *peerStreamLimiter
	fetchStreams     *peerStreamLimiter
	peerSessions     *peerStreamLimiter // MaxConcurrentSessionsPerPeer
	sessions         chan struct{}      // MaxConcurrentSessions tokens

	// ctx is canceled by Close, ending the running sessions.
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// New returns a new rangesync Service of the node overlay, which opens
// streams and disconnects peers through streamer and reads the reserve
// through store. Fetched chunks are validated as pullsync validates them,
// with the same unwrap, gsocHandler and validStamp that pullsync.New gets:
// nil unwrap and gsocHandler do nothing, and a nil validStamp rejects every
// delivery. It starts nothing; see Start.
func New(
	streamer p2p.StreamerDisconnecter,
	store Store,
	overlay swarm.Address,
	unwrap func(swarm.Chunk),
	gsocHandler func(*soc.SOC),
	validStamp postage.ValidStampFn,
	logger log.Logger,
	o Options,
) *Service {
	if o.Scorer == nil {
		o.Scorer = noopScorer{}
	}
	if o.FetchTimeout <= 0 {
		o.FetchTimeout = ClaimDeadline / 2
	}
	if o.FetchRate <= 0 {
		o.FetchRate = DefaultFetchRate
	}
	if unwrap == nil {
		unwrap = func(swarm.Chunk) {}
	}
	if gsocHandler == nil {
		gsocHandler = func(*soc.SOC) {}
	}
	if validStamp == nil {
		validStamp = func(swarm.Chunk) (swarm.Chunk, error) { return nil, errNoStampValidator }
	}
	index := fingerprint.NewIndex(fingerprint.IndexOptions{
		LeafBits:           o.LeafBits,
		RootChangeDebounce: StatusMinInterval,
	})
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		streamer:         streamer,
		store:            store,
		logger:           logger.WithName(loggerName).Register(),
		metrics:          newMetrics(),
		scorer:           o.Scorer,
		wants:            o.Wants,
		unwrap:           unwrap,
		gsocHandler:      gsocHandler,
		validStamp:       validStamp,
		fetchTimeout:     o.FetchTimeout,
		wantSet:          wantset.New(ClaimDeadline),
		fetchLimiter:     ratelimit.New(time.Second/time.Duration(o.FetchRate), MaxWantIDs),
		deliveryLimiter:  ratelimit.New(handleRateLimit, MaxWantIDs),
		sessionLimiter:   ratelimit.New(PeerSessionInterval, PeerSessionBurst),
		statusLimiter:    ratelimit.New(PeerStatusInterval, PeerStatusBurst),
		index:            index,
		indexer:          NewIndexer(index, store, overlay, logger, o.Indexer),
		local:            &local{index: index, store: store},
		hints:            newHintCache(HintCacheSize),
		statuses:         make(map[string]Status),
		statusStreams:    newPeerStreamLimiter(maxPeerStatusStreams),
		reconcileStreams: newPeerStreamLimiter(maxPeerReconcileStreams),
		fetchStreams:     newPeerStreamLimiter(maxPeerFetchStreams),
		peerSessions:     newPeerStreamLimiter(MaxConcurrentSessionsPerPeer),
		sessions:         make(chan struct{}, MaxConcurrentSessions),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start registers the Indexer as the reserve observer and, if that
// succeeded, starts it: the Indexer waits for ReserveAddrIndexReady and
// builds the index. It returns the registration error (for example
// storer.ErrAddrIndexDisabled), in which case rangesync must not be run.
// Until the index is built, sessions are answered with Busy and SendStatus
// fails.
func (s *Service) Start() error {
	if err := s.store.SetReserveObserver(s.indexer); err != nil {
		return fmt.Errorf("register reserve observer: %w", err)
	}
	s.indexer.Start()
	return nil
}

// Protocol returns the p2p protocol specification of rangesync: protocol
// ProtocolName, version ProtocolVersion, with the status, reconcile and fetch
// streams.
func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    ProtocolName,
		Version: ProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    StatusStreamName,
				Handler: s.handleStatus,
			},
			{
				Name:    ReconcileStreamName,
				Handler: s.handleReconcile,
			},
			{
				Name:    FetchStreamName,
				Handler: s.handleFetch,
			},
		},
		DisconnectIn:  s.disconnected,
		DisconnectOut: s.disconnected,
	}
}

// disconnected forgets the peer's status, stream slots, delivery, session
// and status limits.
func (s *Service) disconnected(peer p2p.Peer) error {
	s.statusMu.Lock()
	delete(s.statuses, peer.Address.ByteString())
	s.statusMu.Unlock()
	s.statusStreams.clear(peer.Address)
	s.reconcileStreams.clear(peer.Address)
	s.fetchStreams.clear(peer.Address)
	s.deliveryLimiter.Clear(peer.Address.ByteString())
	s.sessionLimiter.Clear(peer.Address.ByteString())
	s.statusLimiter.Clear(peer.Address.ByteString())
	s.peerSessions.clear(peer.Address)
	return nil
}

// Metrics returns the metrics of the service and its index.
func (s *Service) Metrics() []prometheus.Collector {
	return append(s.metrics.collectors(), s.index.Metrics()...)
}

// Close ends the running sessions, stops the Indexer, closes the index and
// the want set (later fetches request nothing).
// The Indexer stays registered with the reserve: commits after Close are
// still applied to the index.
func (s *Service) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		err = errors.Join(s.indexer.Close(), s.index.Close(), s.wantSet.Close())
	})
	return err
}
