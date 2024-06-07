// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	mockAccounting "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds/factory"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockP2P "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	mockPingPong "github.com/ethersphere/bee/v2/pkg/pingpong/mock"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore"
	mockPost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	mockPostContract "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	mockPushsync "github.com/ethersphere/bee/v2/pkg/pushsync/mock"
	resolverMock "github.com/ethersphere/bee/v2/pkg/resolver/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	mockchequebook "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	mockSteward "github.com/ethersphere/bee/v2/pkg/steward/mock"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	stakingContractMock "github.com/ethersphere/bee/v2/pkg/storageincentives/staking/mock"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	mockTopology "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/ioutil"
	"github.com/hashicorp/go-multierror"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/sync/errgroup"
)

type DevBee struct {
	tracerCloser        io.Closer
	stateStoreCloser    io.Closer
	localstoreCloser    io.Closer
	apiCloser           io.Closer
	pssCloser           io.Closer
	accesscontrolCloser io.Closer
	errorLogWriter      io.Writer
	apiServer           *http.Server
}

type DevOptions struct {
	Logger                   log.Logger
	APIAddr                  string
	CORSAllowedOrigins       []string
	DBOpenFilesLimit         uint64
	ReserveCapacity          uint64
	DBWriteBufferSize        uint64
	DBBlockCacheCapacity     uint64
	DBDisableSeeksCompaction bool
}

// NewDevBee starts the bee instance in 'development' mode
// this implies starting an API and a Debug endpoints while mocking all their services.
func NewDevBee(logger log.Logger, o *DevOptions) (b *DevBee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled: false,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	sink := ioutil.WriterFunc(func(p []byte) (int, error) {
		logger.Error(nil, string(p))
		return len(p), nil
	})

	b = &DevBee{
		errorLogWriter: sink,
		tracerCloser:   tracerCloser,
	}

	stateStore, err := leveldb.NewInMemoryStateStore(logger)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	swarmAddress, err := randomAddress()
	if err != nil {
		return nil, err
	}

	batchStore, err := batchstore.New(stateStore, func(b []byte) error { return nil }, 1000000, logger)
	if err != nil {
		return nil, fmt.Errorf("batchstore: %w", err)
	}

	err = batchStore.PutChainState(&postage.ChainState{
		CurrentPrice: big.NewInt(1),
		TotalAmount:  big.NewInt(1),
	})
	if err != nil {
		return nil, fmt.Errorf("batchstore: %w", err)
	}

	mockKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		return nil, err
	}
	signer := crypto.NewDefaultSigner(mockKey)

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, fmt.Errorf("blockchain address: %w", err)
	}

	var mockTransaction = transactionmock.New(transactionmock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
		return []common.Hash{common.HexToHash("abcd")}, nil
	}), transactionmock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
		return nil
	}), transactionmock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
		recipient := common.HexToAddress("dfff")
		return &transaction.StoredTransaction{
			To:          &recipient,
			Created:     1,
			Data:        []byte{1, 2, 3, 4},
			GasPrice:    big.NewInt(12),
			GasTipBoost: 10,
			GasFeeCap:   big.NewInt(12),
			GasTipCap:   new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), big.NewInt(12)), big.NewInt(100)),
			GasLimit:    5345,
			Value:       big.NewInt(4),
			Nonce:       3,
			Description: "test",
		}, nil
	}), transactionmock.WithCancelTransactionFunc(func(ctx context.Context, originalTxHash common.Hash) (common.Hash, error) {
		return common.Hash{}, nil
	}),
	)

	chainBackend := backendmock.New(
		backendmock.WithBlockNumberFunc(func(ctx context.Context) (uint64, error) {
			return 1, nil
		}),
		backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
			return big.NewInt(0), nil
		}),
	)

	// Create api.Probe in healthy state and switch to ready state after all components have been constructed
	probe := api.NewProbe()
	probe.SetHealthy(api.ProbeStatusOK)
	defer func(probe *api.Probe) {
		if err != nil {
			probe.SetHealthy(api.ProbeStatusNOK)
		} else {
			probe.SetReady(api.ProbeStatusOK)
		}
	}(probe)

	localStore, err := storer.New(context.Background(), "", &storer.Options{
		Logger:        logger,
		CacheCapacity: 1_000_000,
	})
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = localStore

	session := accesscontrol.NewDefaultSession(mockKey)
	actLogic := accesscontrol.NewLogic(session)
	accesscontrol := accesscontrol.NewController(actLogic)
	b.accesscontrolCloser = accesscontrol

	pssService := pss.New(mockKey, logger)
	b.pssCloser = pssService

	pssService.SetPushSyncer(mockPushsync.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		pssService.TryUnwrap(chunk)
		return &pushsync.Receipt{}, nil
	}))

	post := mockPost.New()
	postageContract := mockPostContract.New(
		mockPostContract.WithCreateBatchFunc(
			func(ctx context.Context, amount *big.Int, depth uint8, immutable bool, label string) (common.Hash, []byte, error) {
				id := postagetesting.MustNewID()
				batch := &postage.Batch{
					ID:        id,
					Owner:     overlayEthAddress.Bytes(),
					Value:     big.NewInt(0).Mul(amount, big.NewInt(int64(1<<depth))),
					Depth:     depth,
					Immutable: immutable,
				}

				err := batchStore.Save(batch)
				if err != nil {
					return common.Hash{}, nil, err
				}

				stampIssuer := postage.NewStampIssuer(label, string(overlayEthAddress.Bytes()), id, amount, batch.Depth, 0, 0, immutable)
				_ = post.Add(stampIssuer)

				return common.Hash{}, id, nil
			},
		),
		mockPostContract.WithTopUpBatchFunc(
			func(ctx context.Context, batchID []byte, topupAmount *big.Int) (common.Hash, error) {
				return common.Hash{}, postagecontract.ErrNotImplemented
			},
		),
		mockPostContract.WithDiluteBatchFunc(
			func(ctx context.Context, batchID []byte, newDepth uint8) (common.Hash, error) {
				return common.Hash{}, postagecontract.ErrNotImplemented
			},
		),
	)

	var (
		lightNodes = lightnode.NewContainer(swarm.NewAddress(nil))
		pingPong   = mockPingPong.New(pong)
		p2ps       = mockP2P.New(
			mockP2P.WithConnectFunc(func(ctx context.Context, addr multiaddr.Multiaddr) (address *bzz.Address, err error) {
				return &bzz.Address{}, nil
			}), mockP2P.WithDisconnectFunc(
				func(swarm.Address, string) error {
					return nil
				},
			), mockP2P.WithAddressesFunc(
				func() ([]multiaddr.Multiaddr, error) {
					ma, _ := multiaddr.NewMultiaddr("mock")
					return []multiaddr.Multiaddr{ma}, nil
				},
			))
		acc       = mockAccounting.NewAccounting()
		kad       = mockTopology.NewTopologyDriver()
		pseudoset = pseudosettle.New(nil, logger, stateStore, nil, big.NewInt(10000), big.NewInt(10000), p2ps)
		mockSwap  = swapmock.New(swapmock.WithCashoutStatusFunc(
			func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
				return &chequebook.CashoutStatus{
					Last:           &chequebook.LastCashout{},
					UncashedAmount: big.NewInt(0),
				}, nil
			},
		), swapmock.WithLastSentChequeFunc(
			func(a swarm.Address) (*chequebook.SignedCheque, error) {
				return &chequebook.SignedCheque{
					Cheque: chequebook.Cheque{
						Beneficiary: common.Address{},
						Chequebook:  common.Address{},
					},
				}, nil
			},
		), swapmock.WithLastReceivedChequeFunc(
			func(a swarm.Address) (*chequebook.SignedCheque, error) {
				return &chequebook.SignedCheque{
					Cheque: chequebook.Cheque{
						Beneficiary: common.Address{},
						Chequebook:  common.Address{},
					},
				}, nil
			},
		))
		mockChequebook = mockchequebook.NewChequebook(mockchequebook.WithChequebookBalanceFunc(
			func(context.Context) (ret *big.Int, err error) {
				return big.NewInt(0), nil
			},
		), mockchequebook.WithChequebookAvailableBalanceFunc(
			func(context.Context) (ret *big.Int, err error) {
				return big.NewInt(0), nil
			},
		), mockchequebook.WithChequebookWithdrawFunc(
			func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
				return common.Hash{}, nil
			},
		), mockchequebook.WithChequebookDepositFunc(
			func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
				return common.Hash{}, nil
			},
		))
	)

	var (
		// syncStatusFn mocks sync status because complete sync is required in order to curl certain apis e.g. /stamps.
		// this allows accessing those apis by passing true to isDone in devNode.
		syncStatusFn = func() (isDone bool, err error) {
			return true, nil
		}
	)

	mockFeeds := factory.New(localStore.Download(true))
	mockResolver := resolverMock.NewResolver()
	mockSteward := new(mockSteward.Steward)

	mockStaking := stakingContractMock.New(
		stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
			return common.Hash{}, staking.ErrNotImplemented
		}),
		stakingContractMock.WithGetStake(func(ctx context.Context) (*big.Int, error) {
			return nil, staking.ErrNotImplemented
		}),
		stakingContractMock.WithWithdrawAllStake(func(ctx context.Context) (common.Hash, error) {
			return common.Hash{}, staking.ErrNotImplemented
		}),
		stakingContractMock.WithIsFrozen(func(ctx context.Context, block uint64) (bool, error) {
			return false, staking.ErrNotImplemented
		}),
	)

	debugOpts := api.ExtraOptions{
		Pingpong:        pingPong,
		TopologyDriver:  kad,
		LightNodes:      lightNodes,
		Accounting:      acc,
		Pseudosettle:    pseudoset,
		Swap:            mockSwap,
		Chequebook:      mockChequebook,
		BlockTime:       time.Second * 2,
		Storer:          localStore,
		Resolver:        mockResolver,
		Pss:             pssService,
		FeedFactory:     mockFeeds,
		Post:            post,
		AccessControl:   accesscontrol,
		PostageContract: postageContract,
		Staking:         mockStaking,
		Steward:         mockSteward,
		SyncStatus:      syncStatusFn,
	}

	var erc20 = erc20mock.New(
		erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
			return big.NewInt(0), nil
		}),
		erc20mock.WithTransferFunc(func(ctx context.Context, address common.Address, value *big.Int) (common.Hash, error) {
			return common.Hash{}, nil
		}),
	)

	apiService := api.New(mockKey.PublicKey, mockKey.PublicKey, overlayEthAddress, nil, logger, mockTransaction, batchStore, api.DevMode, true, true, chainBackend, o.CORSAllowedOrigins, inmemstore.New())

	apiService.Configure(signer, tracer, api.Options{
		CORSAllowedOrigins: o.CORSAllowedOrigins,
		WsPingPeriod:       60 * time.Second,
	}, debugOpts, 1, erc20)
	apiService.MountTechnicalDebug()
	apiService.MountDebug()
	apiService.MountAPI()

	apiService.SetProbe(probe)
	apiService.SetP2P(p2ps)
	apiService.SetSwarmAddress(&swarmAddress)

	apiListener, err := net.Listen("tcp", o.APIAddr)
	if err != nil {
		return nil, fmt.Errorf("api listener: %w", err)
	}

	apiServer := &http.Server{
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           apiService,
		ErrorLog:          stdlog.New(b.errorLogWriter, "", 0),
	}
	go func() {
		logger.Info("starting api server", "address", apiListener.Addr())

		if err := apiServer.Serve(apiListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Debug("api server failed to start", "error", err)
			logger.Error(nil, "api server failed to start")
		}
	}()

	b.apiServer = apiServer
	b.apiCloser = apiService

	return b, nil
}

func (b *DevBee) Shutdown() error {
	var mErr error

	tryClose := func(c io.Closer, errMsg string) {
		if c == nil {
			return
		}
		if err := c.Close(); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("%s: %w", errMsg, err))
		}
	}

	tryClose(b.apiCloser, "api")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eg errgroup.Group
	if b.apiServer != nil {
		eg.Go(func() error {
			if err := b.apiServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("api server: %w", err)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	tryClose(b.pssCloser, "pss")
	tryClose(b.accesscontrolCloser, "accesscontrol")
	tryClose(b.tracerCloser, "tracer")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")

	return mErr
}

func pong(_ context.Context, _ swarm.Address, _ ...string) (rtt time.Duration, err error) {
	return time.Millisecond, nil
}

func randomAddress() (swarm.Address, error) {

	b := make([]byte, 32)

	_, err := rand.Read(b)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(b), nil

}
