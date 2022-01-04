package node

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/priceoracle"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/kademlia"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

var snapshotReference = swarm.MustParseHexAddress("b933cd26548ae992e0ee6ba6fabdf9ab7769663d8ddac84b65b1a527ea0734e7")

func NewBeeBootstrapper(addr string, publicKey *ecdsa.PublicKey, signer crypto.Signer, networkID uint64, logger logging.Logger, libp2pPrivateKey, pssPrivateKey *ecdsa.PrivateKey, o *Options) (b *Bee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled:     o.TracingEnabled,
		Endpoint:    o.TracingEndpoint,
		ServiceName: o.TracingServiceName,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	p2pCtx, p2pCancel := context.WithCancel(context.Background())
	defer func() {
		// if there's been an error on this function
		// we'd like to cancel the p2p context so that
		// incoming connections will not be possible
		if err != nil {
			p2pCancel()
		}
	}()

	b = &Bee{
		p2pCancel:      p2pCancel,
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	stateStore, err := InitStateStore(logger, o.DataDir)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	addressbook := addressbook.New(stateStore)

	var (
		swapBackend        transaction.Backend
		overlayEthAddress  common.Address
		chainID            int64
		transactionService transaction.Service
		transactionMonitor transaction.Monitor
		chequebookFactory  chequebook.Factory
		chequebookService  chequebook.Service
		chequeStore        chequebook.ChequeStore
		cashoutService     chequebook.CashoutService
		pollingInterval    = time.Duration(o.BlockTime) * time.Second
	)
	swapBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err = InitChain(
		p2pCtx,
		logger,
		stateStore,
		o.SwapEndpoint,
		signer,
		pollingInterval,
	)
	if err != nil {
		return nil, fmt.Errorf("init chain: %w", err)
	}
	b.ethClientCloser = swapBackend.Close
	b.transactionCloser = tracerCloser
	b.transactionMonitorCloser = transactionMonitor

	if o.ChainID != -1 && o.ChainID != chainID {
		return nil, fmt.Errorf("connected to wrong ethereum network; network chainID %d; configured chainID %d", chainID, o.ChainID)
	}

	// Sync the with the given Ethereum backend:
	isSynced, _, err := transaction.IsSynced(p2pCtx, swapBackend, maxDelay)
	if err != nil {
		return nil, fmt.Errorf("is synced: %w", err)
	}
	if !isSynced {
		logger.Infof("waiting to sync with the Ethereum backend")

		err := transaction.WaitSynced(p2pCtx, logger, swapBackend, maxDelay)
		if err != nil {
			return nil, fmt.Errorf("waiting backend sync: %w", err)
		}
	}

	if o.SwapEnable {
		chequebookFactory, err = InitChequebookFactory(
			logger,
			swapBackend,
			chainID,
			transactionService,
			o.SwapFactoryAddress,
			o.SwapLegacyFactoryAddresses,
		)
		if err != nil {
			return nil, err
		}

		if err = chequebookFactory.VerifyBytecode(p2pCtx); err != nil {
			return nil, fmt.Errorf("factory fail: %w", err)
		}

		if o.ChequebookEnable {
			chequebookService, err = InitChequebookService(
				p2pCtx,
				logger,
				stateStore,
				signer,
				chainID,
				swapBackend,
				overlayEthAddress,
				transactionService,
				chequebookFactory,
				o.SwapInitialDeposit,
				o.DeployGasPrice,
			)
			if err != nil {
				return nil, err
			}
		}

		chequeStore, cashoutService = initChequeStoreCashout(
			stateStore,
			swapBackend,
			chequebookFactory,
			chainID,
			overlayEthAddress,
			transactionService,
		)
	}

	pubKey, _ := signer.PublicKey()
	if err != nil {
		return nil, err
	}

	var (
		blockHash []byte
		txHash    []byte
	)

	txHash, err = GetTxHash(stateStore, logger, o.Transaction)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction hash: %w", err)
	}

	blockHash, err = GetTxNextBlock(p2pCtx, logger, swapBackend, transactionMonitor, pollingInterval, txHash, o.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("invalid block hash: %w", err)
	}

	swarmAddress, err := crypto.NewOverlayAddress(*pubKey, networkID, blockHash)

	err = CheckOverlayWithStore(swarmAddress, stateStore)
	if err != nil {
		return nil, err
	}

	lightNodes := lightnode.NewContainer(swarmAddress)

	senderMatcher := transaction.NewMatcher(swapBackend, types.NewLondonSigner(big.NewInt(chainID)), stateStore)

	_, err = senderMatcher.Matches(p2pCtx, txHash, networkID, swarmAddress, true)
	if err != nil {
		return nil, fmt.Errorf("identity transaction verification failed: %w", err)
	}

	p2ps, err := libp2p.New(p2pCtx, signer, networkID, swarmAddress, addr, addressbook, stateStore, lightNodes, senderMatcher, logger, tracer, libp2p.Options{
		PrivateKey:     libp2pPrivateKey,
		NATAddr:        o.NATAddr,
		EnableWS:       o.EnableWS,
		WelcomeMessage: o.WelcomeMessage,
		FullNode:       o.FullNodeMode,
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

	var bootnodes []ma.Multiaddr

	for _, a := range o.Bootnodes {
		addr, err := ma.NewMultiaddr(a)
		if err != nil {
			logger.Debugf("multiaddress fail %s: %v", a, err)
			logger.Warningf("invalid bootnode address %s", a)
			continue
		}

		bootnodes = append(bootnodes, addr)
	}

	var swapService *swap.Service

	metricsDB, err := shed.NewDBWrap(stateStore.DB())
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics storage for kademlia: %w", err)
	}

	kad, err := kademlia.New(swarmAddress, addressbook, hive, p2ps, nil /* TODO: pingpong is nil here. we need a noop pingpong */, metricsDB, logger,
		kademlia.Options{Bootnodes: bootnodes, BootnodeMode: o.BootnodeMode, StaticNodes: o.StaticNodes})
	if err != nil {
		return nil, fmt.Errorf("unable to create kademlia: %w", err)
	}
	b.topologyCloser = kad
	b.topologyHalter = kad
	hive.SetAddPeersHandler(kad.AddPeers)
	p2ps.SetPickyNotifier(kad)

	minThreshold := big.NewInt(2 * refreshRate)
	maxThreshold := big.NewInt(24 * refreshRate)

	paymentThreshold, ok := new(big.Int).SetString(o.PaymentThreshold, 10)
	if !ok {
		return nil, fmt.Errorf("invalid payment threshold: %s", paymentThreshold)
	}

	pricer := pricer.NewFixedPricer(swarmAddress, basePrice)

	if paymentThreshold.Cmp(minThreshold) < 0 {
		return nil, fmt.Errorf("payment threshold below minimum generally accepted value, need at least %s", minThreshold)
	}

	if paymentThreshold.Cmp(maxThreshold) > 0 {
		return nil, fmt.Errorf("payment threshold above maximum generally accepted value, needs to be reduced to at most %s", maxThreshold)
	}

	pricing := pricing.New(p2ps, logger, paymentThreshold, minThreshold)

	if err = p2ps.AddProtocol(pricing.Protocol()); err != nil {
		return nil, fmt.Errorf("pricing service: %w", err)
	}

	addrs, err := p2ps.Addresses()
	if err != nil {
		return nil, fmt.Errorf("get server addresses: %w", err)
	}

	for _, addr := range addrs {
		logger.Debugf("p2p address: %s", addr)
	}

	if o.PaymentTolerance < 0 {
		return nil, fmt.Errorf("invalid payment tolerance: %d", o.PaymentTolerance)
	}

	if o.PaymentEarly > 100 || o.PaymentEarly < 0 {
		return nil, fmt.Errorf("invalid payment early: %d", o.PaymentEarly)
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

	var enforcedRefreshRate *big.Int

	if o.FullNodeMode {
		enforcedRefreshRate = big.NewInt(refreshRate)
	} else {
		enforcedRefreshRate = big.NewInt(lightRefreshRate)
	}

	pseudosettleService := pseudosettle.New(p2ps, logger, stateStore, acc, enforcedRefreshRate, big.NewInt(lightRefreshRate), p2ps)
	if err = p2ps.AddProtocol(pseudosettleService.Protocol()); err != nil {
		return nil, fmt.Errorf("pseudosettle service: %w", err)
	}

	acc.SetRefreshFunc(pseudosettleService.Pay)

	var priceOracle priceoracle.Service
	swapService, priceOracle, err = InitSwap(
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
	p2ps.Ready()

	if !waitPeers(kad) {
		return nil, errors.New("timed out waiting for kademlia peers")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fmt.Println("bootstrapper trying to fetch file")
	reader, l, err := joiner.New(ctx, ns, snapshotReference)
	if err != nil {
		panic(err)
	}

	f, err := os.CreateTemp("", "swarm_snapshot")
	if err != nil {
		panic(err)
	}
	fmt.Println("using temp file at", f.Name())
	n, err := io.Copy(f, reader)
	if err != nil {
		panic(err)
	}
	if n != l {
		panic("short write")
	}

	fmt.Println("thats it")

	return b, nil
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
