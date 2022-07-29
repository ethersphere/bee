// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/feeds/factory"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/kademlia"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/hashicorp/go-multierror"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	snapshotFeed    = swarm.MustParseHexAddress("b181b084df07a550c9fc0007110bff67000fa92a090af6c5212fe8e19f888a28")
	errDataMismatch = errors.New("data length mismatch")
)

const (
	getSnapshotRetries = 3
	retryWait          = time.Second * 5
	timeout            = time.Minute * 2
)

func bootstrapNode(
	addr string,
	swarmAddress swarm.Address,
	txHash []byte,
	chainID int64,
	overlayEthAddress common.Address,
	addressbook addressbook.Interface,
	bootnodes []ma.Multiaddr,
	lightNodes *lightnode.Container,
	senderMatcher p2p.SenderMatcher,
	chequebookService chequebook.Service,
	chequeStore chequebook.ChequeStore,
	cashoutService chequebook.CashoutService,
	transactionService transaction.Service,
	stateStore storage.StateStorer,
	signer crypto.Signer,
	networkID uint64,
	logger logging.Logger,
	libp2pPrivateKey *ecdsa.PrivateKey,
	o *Options,
) (snapshot *postage.ChainSnapshot, retErr error) {

	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	p2pCtx, p2pCancel := context.WithCancel(context.Background())

	b := &Bee{
		p2pCancel:    p2pCancel,
		tracerCloser: tracerCloser,
	}

	defer func() {
		retErr = multierror.Append(new(multierror.Error), retErr, b.Shutdown()).ErrorOrNil()
	}()

	p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, senderMatcher, logger, tracer, libp2p.Options{
		PrivateKey:     libp2pPrivateKey,
		NATAddr:        o.NATAddr,
		EnableWS:       o.EnableWS,
		WelcomeMessage: o.WelcomeMessage,
		FullNode:       false,
		Transaction:    txHash,
	})
	if err != nil {
		return nil, fmt.Errorf("p2p service: %w", err)
	}
	b.p2pService = p2ps
	b.p2pHalter = p2ps

	hive, err := hive.New(p2ps, addressbook, networkID, o.BootnodeMode, o.AllowPrivateCIDRs, logger)
	if err != nil {
		return nil, fmt.Errorf("hive: %w", err)
	}

	if err = p2ps.AddProtocol(hive.Protocol()); err != nil {
		return nil, fmt.Errorf("hive service: %w", err)
	}
	b.hiveCloser = hive

	metricsDB, err := shed.NewDBWrap(stateStore.DB())
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics storage for kademlia: %w", err)
	}

	kad, err := kademlia.New(swarmAddress, addressbook, hive, p2ps, &noopPinger{}, metricsDB, logger,
		kademlia.Options{Bootnodes: bootnodes, BootnodeMode: o.BootnodeMode, StaticNodes: o.StaticNodes})
	if err != nil {
		return nil, fmt.Errorf("unable to create kademlia: %w", err)
	}
	b.topologyCloser = kad
	b.topologyHalter = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetPickyNotifier(kad)

	paymentThreshold, _ := new(big.Int).SetString(o.PaymentThreshold, 10)

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	pricing := pricing.New(p2ps, logger, paymentThreshold, big.NewInt(minPaymentThreshold))
	if err = p2ps.AddProtocol(pricing.Protocol()); err != nil {
		return nil, fmt.Errorf("pricing service: %w", err)
	}

	acc, err := accounting.NewAccounting(
		paymentThreshold,
		o.PaymentTolerance,
		o.PaymentEarly,
		logger,
		stateStore,
		pricing,
		big.NewInt(refreshRate),
		p2ps,
	)
	if err != nil {
		return nil, fmt.Errorf("accounting: %w", err)
	}
	b.accountingCloser = acc

	// bootstraper mode uses the light node refresh rate
	enforcedRefreshRate := big.NewInt(lightRefreshRate)

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, enforcedRefreshRate, enforcedRefreshRate, p2ps)
	if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
		return nil, fmt.Errorf("pseudosettle service: %w", err)
	}

	acc.SetRefreshFunc(pseudosettleService.Pay)

	pricing.SetPaymentThresholdObserver(acc)

	noopValidStamp := func(chunk swarm.Chunk, _ []byte) (swarm.Chunk, error) {
		return chunk, nil
	}

	storer := inmemstore.New()

	retrieve := retrieval.New(swarmAddress, storer, p2ps, kad, logger, acc, pricer, tracer, o.RetrievalCaching, noopValidStamp)
	if err = p2ps.AddProtocol(retrieve.Protocol()); err != nil {
		return nil, fmt.Errorf("retrieval service: %w", err)
	}

	ns := netstore.New(storer, noopValidStamp, retrieve, logger)

	if err := kad.Start(p2pCtx); err != nil {
		return nil, err
	}

	if err := p2ps.Ready(); err != nil {
		return nil, err
	}

	if !waitPeers(kad) {
		return nil, errors.New("timed out waiting for kademlia peers")
	}

	logger.Info("bootstrap: trying to fetch stamps snapshot")

	var (
		snapshotReference swarm.Address
		reader            file.Joiner
		l                 int64
		eventsJSON        []byte
	)

	for i := 0; i < getSnapshotRetries; i++ {
		if err != nil {
			time.Sleep(retryWait)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		snapshotReference, err = getLatestSnapshot(ctx, ns, snapshotFeed)
		if err != nil {
			logger.Warningf("bootstrap: fetching snapshot: %v", err)
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}

	for i := 0; i < getSnapshotRetries; i++ {
		if err != nil {
			time.Sleep(retryWait)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		reader, l, err = joiner.New(ctx, ns, snapshotReference)
		if err != nil {
			logger.Warningf("bootstrap: file joiner: %v", err)
			continue
		}

		eventsJSON, err = io.ReadAll(reader)
		if err != nil {
			logger.Warningf("bootstrap: reading: %v", err)
			continue
		}

		if len(eventsJSON) != int(l) {
			err = errDataMismatch
			logger.Warningf("bootstrap: %v", err)
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}

	events := postage.ChainSnapshot{}
	err = json.Unmarshal(eventsJSON, &events)
	if err != nil {
		return nil, err
	}

	return &events, nil
}

// wait till some peers are connected. returns true if all is ok
func waitPeers(kad *kademlia.Kad) bool {
	for i := 0; i < 60; i++ {
		items := 0
		_ = kad.EachPeer(func(_ swarm.Address, _ uint8) (bool, bool, error) {
			items++
			return false, false, nil
		}, topology.Filter{})
		if items >= 25 {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

type noopPinger struct{}

func (p *noopPinger) Ping(context.Context, swarm.Address, ...string) (time.Duration, error) {
	return time.Duration(1), nil
}

func getLatestSnapshot(
	ctx context.Context,
	st storage.Storer,
	address swarm.Address,
) (swarm.Address, error) {
	ls := loadsave.NewReadonly(st)
	feedFactory := factory.New(st)

	m, err := manifest.NewDefaultManifestReference(
		address,
		ls,
	)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("not a manifest: %w", err)
	}

	e, err := m.Lookup(ctx, "/")
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("node lookup: %w", err)
	}

	var (
		owner, topic []byte
		t            = new(feeds.Type)
	)
	meta := e.Metadata()
	if e := meta["swarm-feed-owner"]; e != "" {
		owner, err = hex.DecodeString(e)
		if err != nil {
			return swarm.ZeroAddress, err
		}
	}
	if e := meta["swarm-feed-topic"]; e != "" {
		topic, err = hex.DecodeString(e)
		if err != nil {
			return swarm.ZeroAddress, err
		}
	}
	if e := meta["swarm-feed-type"]; e != "" {
		err := t.FromString(e)
		if err != nil {
			return swarm.ZeroAddress, err
		}
	}
	if len(owner) == 0 || len(topic) == 0 {
		return swarm.ZeroAddress, fmt.Errorf("node lookup: %s", "feed metadata absent")
	}
	f := feeds.New(topic, common.BytesToAddress(owner))

	l, err := feedFactory.NewLookup(*t, f)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("feed lookup failed: %w", err)
	}

	u, _, _, err := l.At(ctx, time.Now().Unix(), 0)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	_, ref, err := feeds.FromChunk(u)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(ref), nil
}

func batchStoreExists(s storage.StateStorer) (bool, error) {

	hasOne := false
	err := s.Iterate("batchstore_", func(key, value []byte) (stop bool, err error) {
		hasOne = true
		return true, err
	})

	return hasOne, err
}
