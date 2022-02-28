package node

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/logging"
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
	"github.com/sirupsen/logrus"
)

var snapshotReference = swarm.MustParseHexAddress("b36f03d995a04df1757c3a5ddbb795f48d279c532b11803864503f6b97fb20e1")

func bootstrapNode(addr string,
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
	stateStore storage.StateStorer, publicKey *ecdsa.PublicKey, signer crypto.Signer, networkID uint64, logger logging.Logger, libp2pPrivateKey, pssPrivateKey *ecdsa.PrivateKey, o *Options) (snapshot *postage.BatchSnapshot, retErr error) {

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
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := multierror.Append(new(multierror.Error), retErr, b.Shutdown(ctx))
		retErr = err.ErrorOrNil()
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

	minThreshold := big.NewInt(2 * refreshRate)
	paymentThreshold, _ := new(big.Int).SetString(o.PaymentThreshold, 10)

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	pricing := pricing.New(p2ps, logger, paymentThreshold, minThreshold)

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

	enforcedRefreshRate := big.NewInt(lightRefreshRate)

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, enforcedRefreshRate, big.NewInt(lightRefreshRate), p2ps)
	if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
		return nil, fmt.Errorf("pseudosettle service: %w", err)
	}

	acc.SetRefreshFunc(pseudosettleService.Pay)

	swapService, priceOracle, err := InitSwap(
		p2ps,
		logger,
		stateStore,
		networkID,
		overlayEthAddress,
		chequebookService,
		chequeStore,
		cashoutService,
		acc,
		o.PriceOracleAddress,
		chainID,
		transactionService,
	)
	if err != nil {
		return nil, err
	}
	b.priceOracleCloser = priceOracle

	if o.ChequebookEnable {
		acc.SetPayFunc(swapService.Pay)
	}

	pricing.SetPaymentThresholdObserver(acc)

	noopValidStamp := func(chunk swarm.Chunk, _ []byte) (swarm.Chunk, error) {
		return chunk, nil
	}

	storer := inmemstore.New()
	retrieve := retrieval.New(swarmAddress, storer, p2ps, kad, logger, acc, pricer, tracer, o.RetrievalCaching, noopValidStamp)
	ns := netstore.New(storer, noopValidStamp, nil, retrieve, logger)

	retrieveProtocolSpec := retrieve.Protocol()

	if err = p2ps.AddProtocol(retrieveProtocolSpec); err != nil {
		return nil, fmt.Errorf("retrieval service: %w", err)
	}

	if err := kad.Start(p2pCtx); err != nil {
		return nil, err
	}

	if err := p2ps.Ready(); err != nil {
		return nil, err
	}

	if !waitPeers(kad) {
		return nil, errors.New("timed out waiting for kademlia peers")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("bootstrap: trying to fetch stamps snapshot")
	reader, l, err := joiner.New(ctx, ns, snapshotReference)
	if err != nil {
		return nil, err
	}

	eventsJSON, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if len(eventsJSON) != int(l) {
		return nil, err
	}

	events := postage.BatchSnapshot{}
	err = json.Unmarshal(eventsJSON, &events)
	if err != nil {
		return nil, err
	}

	return &events, nil
}

// wait till some peers are connected. returns true if all is ok
func waitPeers(kad *kademlia.Kad) bool {
	items := 0
	c := make(chan struct{})
	go func() {
		time.After(10 * time.Second)
		close(c)
	}()

	defer func() {
		<-c
	}()

	for i := 0; i < 30; i++ {
		items = 0
		_ = kad.EachPeer(func(_ swarm.Address, _ uint8) (bool, bool, error) {
			items++
			return false, false, nil
		}, topology.Filter{Reachable: false})
		if items >= 5 {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

type noopPinger struct {
}

func (p *noopPinger) Ping(_ context.Context, _ swarm.Address, _ ...string) (time.Duration, error) {
	return time.Duration(1), nil
}
