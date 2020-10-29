// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	mockAccounting "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/feeds/factory"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	mockP2P "github.com/ethersphere/bee/pkg/p2p/mock"
	mockPingPong "github.com/ethersphere/bee/pkg/pingpong/mock"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	mockPost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	mockPostContract "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	mockPushsync "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	mockchequebook "github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
	swapmock "github.com/ethersphere/bee/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	mockStateStore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	mockTopology "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/hashicorp/go-multierror"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type DevBee struct {
	tracerCloser     io.Closer
	stateStoreCloser io.Closer
	localstoreCloser io.Closer
	apiCloser        io.Closer
	pssCloser        io.Closer
	tagsCloser       io.Closer
	errorLogWriter   *io.PipeWriter
	apiServer        *http.Server
	debugAPIServer   *http.Server
}

type DevOptions struct {
	Logger                   logging.Logger
	APIAddr                  string
	DebugAPIAddr             string
	CORSAllowedOrigins       []string
	DBOpenFilesLimit         uint64
	ReserveCapacity          uint64
	DBWriteBufferSize        uint64
	DBBlockCacheCapacity     uint64
	DBDisableSeeksCompaction bool
}

// NewDevBee starts the bee instance in 'development' mode
// this implies starting an API and a Debug endpoints while mocking all their services.
func NewDevBee(logger logging.Logger, o *DevOptions) (b *DevBee, err error) {
	tracer, tracerCloser, err := tracing.NewTracer(&tracing.Options{
		Enabled: false,
	})
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}

	b = &DevBee{
		errorLogWriter: logger.WriterLevel(logrus.ErrorLevel),
		tracerCloser:   tracerCloser,
	}

	stateStore, err := leveldb.NewInMemoryStateStore(logger)
	if err != nil {
		return nil, err
	}
	b.stateStoreCloser = stateStore

	mockKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		return nil, err
	}
	signer := crypto.NewDefaultSigner(mockKey)

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, fmt.Errorf("eth address: %w", err)
	}

	var debugAPIService *debugapi.Service

	if o.DebugAPIAddr != "" {
		debugAPIListener, err := net.Listen("tcp", o.DebugAPIAddr)
		if err != nil {
			return nil, fmt.Errorf("debug api listener: %w", err)
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
				GasLimit:    5345,
				Value:       big.NewInt(4),
				Nonce:       3,
				Description: "test",
			}, nil
		}), transactionmock.WithCancelTransactionFunc(func(ctx context.Context, originalTxHash common.Hash) (common.Hash, error) {
			return common.Hash{}, nil
		}),
		)

		debugAPIService = debugapi.New(mockKey.PublicKey, mockKey.PublicKey, overlayEthAddress, logger, tracer, nil, big.NewInt(0), mockTransaction)

		debugAPIServer := &http.Server{
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           debugAPIService,
			ErrorLog:          log.New(b.errorLogWriter, "", 0),
		}

		go func() {
			logger.Infof("debug api address: %s", debugAPIListener.Addr())

			if err := debugAPIServer.Serve(debugAPIListener); err != nil && err != http.ErrServerClosed {
				logger.Debugf("debug api server: %v", err)
				logger.Error("unable to serve debug api")
			}
		}()

		b.debugAPIServer = debugAPIServer
	}

	lo := &localstore.Options{
		Capacity:               1000,
		ReserveCapacity:        o.ReserveCapacity,
		OpenFilesLimit:         o.DBOpenFilesLimit,
		BlockCacheCapacity:     o.DBBlockCacheCapacity,
		WriteBufferSize:        o.DBWriteBufferSize,
		DisableSeeksCompaction: o.DBDisableSeeksCompaction,
		UnreserveFunc: func(postage.UnreserveIteratorFn) error {
			return nil
		},
	}

	var swarmAddress swarm.Address
	storer, err := localstore.New("", swarmAddress.Bytes(), stateStore, lo, logger)
	if err != nil {
		return nil, fmt.Errorf("localstore: %w", err)
	}
	b.localstoreCloser = storer

	tagService := tags.NewTags(stateStore, logger)
	b.tagsCloser = tagService

	pssService := pss.New(mockKey, logger)
	b.pssCloser = pssService

	pssService.SetPushSyncer(mockPushsync.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		pssService.TryUnwrap(chunk)
		return &pushsync.Receipt{}, nil
	}))

	traversalService := traversal.New(storer)

	pinningService := pinning.NewService(storer, stateStore, traversalService)

	batchStore, err := batchstore.New(stateStore, func(b []byte) error { return nil }, logger)
	if err != nil {
		return nil, fmt.Errorf("batchstore: %w", err)
	}

	post := mockPost.New()
	postageContract := mockPostContract.New(
		mockPostContract.WithCreateBatchFunc(
			func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error) {
				id := postagetesting.MustNewID()
				b := &postage.Batch{
					ID:        id,
					Owner:     overlayEthAddress.Bytes(),
					Value:     big.NewInt(0),
					Depth:     depth,
					Immutable: immutable,
				}

				totalAmount := big.NewInt(0).Mul(initialBalance, big.NewInt(int64(1<<depth)))

				err := batchStore.Put(b, totalAmount, depth)
				if err != nil {
					return nil, err
				}

				stampIssuer := postage.NewStampIssuer(label, string(overlayEthAddress.Bytes()), id, totalAmount, depth, 0, 0, immutable)
				post.Add(stampIssuer)

				return id, nil
			},
		),
		mockPostContract.WithTopUpBatchFunc(
			func(ctx context.Context, batchID []byte, topupAmount *big.Int) error {
				batch, err := batchStore.Get(batchID)
				if err != nil {
					return err
				}

				totalAmount := big.NewInt(0).Mul(topupAmount, big.NewInt(int64(1<<batch.Depth)))

				newBalance := big.NewInt(0).Add(totalAmount, batch.Value)

				err = batchStore.Put(batch, newBalance, batch.Depth)
				if err != nil {
					return err
				}

				post.HandleTopUp(batch.ID, newBalance)
				return nil
			},
		),
		mockPostContract.WithDiluteBatchFunc(
			func(ctx context.Context, batchID []byte, newDepth uint8) error {
				batch, err := batchStore.Get(batchID)
				if err != nil {
					return err
				}

				if newDepth < batch.Depth {
					return postagecontract.ErrInvalidDepth
				}

				newBalance := big.NewInt(0).Div(batch.Value, big.NewInt(int64(1<<(newDepth-batch.Depth))))

				err = batchStore.Put(batch, newBalance, newDepth)
				if err != nil {
					return err
				}

				post.HandleDepthIncrease(batch.ID, newDepth, newBalance)
				return nil
			},
		),
	)

	feedFactory := factory.New(storer)

	apiService := api.New(tagService, storer, nil, nil, pssService, traversalService, pinningService, feedFactory, post, postageContract, nil, signer, logger, tracer, api.Options{
		CORSAllowedOrigins: o.CORSAllowedOrigins,
		GatewayMode:        false,
		WsPingPeriod:       60 * time.Second,
	})

	apiListener, err := net.Listen("tcp", o.APIAddr)
	if err != nil {
		return nil, fmt.Errorf("api listener: %w", err)
	}

	apiServer := &http.Server{
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           apiService,
		ErrorLog:          log.New(b.errorLogWriter, "", 0),
	}

	go func() {
		logger.Infof("api address: %s", apiListener.Addr())

		if err := apiServer.Serve(apiListener); err != nil && err != http.ErrServerClosed {
			logger.Debugf("api server: %v", err)
			logger.Error("unable to serve api")
		}
	}()

	b.apiServer = apiServer
	b.apiCloser = apiService

	if debugAPIService != nil {
		var (
			lightNodes = lightnode.NewContainer(swarm.NewAddress(nil))
			pingPong   = mockPingPong.New(pong)
			p2ps       = mockP2P.New(
				mockP2P.WithConnectFunc(func(ctx context.Context, addr multiaddr.Multiaddr) (address *bzz.Address, err error) {
					return &bzz.Address{}, nil
				}), mockP2P.WithDisconnectFunc(
					func(overlay swarm.Address) error {
						return nil
					},
				), mockP2P.WithAddressesFunc(
					func() ([]multiaddr.Multiaddr, error) {
						ma, _ := multiaddr.NewMultiaddr("mock")
						return []multiaddr.Multiaddr{ma}, nil
					},
				))
			acc            = mockAccounting.NewAccounting()
			kad            = mockTopology.NewTopologyDriver()
			storeRecipient = mockStateStore.NewStateStore()
			pseudoset      = pseudosettle.New(nil, logger, storeRecipient, nil, big.NewInt(10000), p2ps)
			mockSwap       = swapmock.New(swapmock.WithCashoutStatusFunc(
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

		// inject dependencies and configure full debug api http path routes
		debugAPIService.Configure(swarmAddress, p2ps, pingPong, kad, lightNodes, storer, tagService, acc, pseudoset, true, mockSwap, mockChequebook, batchStore, post, postageContract, traversalService)
	}

	return b, nil
}

func (b *DevBee) Shutdown(ctx context.Context) error {
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

	var eg errgroup.Group
	if b.apiServer != nil {
		eg.Go(func() error {
			if err := b.apiServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("api server: %w", err)
			}
			return nil
		})
	}
	if b.debugAPIServer != nil {
		eg.Go(func() error {
			if err := b.debugAPIServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("debug api server: %w", err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	tryClose(b.pssCloser, "pss")
	tryClose(b.tracerCloser, "tracer")
	tryClose(b.tagsCloser, "tag persistence")
	tryClose(b.stateStoreCloser, "statestore")
	tryClose(b.localstoreCloser, "localstore")
	tryClose(b.errorLogWriter, "error log writer")

	return mErr
}

func pong(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	return time.Millisecond, nil
}
