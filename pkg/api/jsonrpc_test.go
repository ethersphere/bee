// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagecontractmock "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	chequebookmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	stakingmock "github.com/ethersphere/bee/v2/pkg/storageincentives/staking/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	backendmock "github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/multiformats/go-multiaddr"
)

func TestJSONRPC(t *testing.T) {

	// Define keys and overlay
	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	pssPrivateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	overlay := swarm.MustParseHexAddress("ca11957861758380")

	// Define P2P service mock
	p2pService := p2pmock.New(
		p2pmock.WithPeersFunc(func() []p2p.Peer {
			return []p2p.Peer{{Address: swarm.MustParseHexAddress("ca11957861758381")}}
		}),
		p2pmock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return []multiaddr.Multiaddr{}, nil
		}),
		p2pmock.WithBlocklistedPeersFunc(func() ([]p2p.BlockListedPeer, error) {
			return []p2p.BlockListedPeer{
				{
					Peer:     p2p.Peer{Address: swarm.MustParseHexAddress("ca11957861758382")},
					Reason:   "test",
					Duration: 10 * time.Second,
				},
			}, nil
		}),
		p2pmock.WithConnectFunc(func(ctx context.Context, addr []multiaddr.Multiaddr) (*bzz.Address, error) {
			return &bzz.Address{Overlay: swarm.MustParseHexAddress("ca11957861758383")}, nil
		}),
		p2pmock.WithDisconnectFunc(func(addr swarm.Address, reason string) error {
			return nil
		}),
	)

	// Define Accounting mock
	accountingOpts := []accountingmock.Option{
		accountingmock.WithBalancesFunc(func() (map[string]*big.Int, error) {
			return map[string]*big.Int{
				"ca11957861758381": big.NewInt(100),
			}, nil
		}),
	}

	// Define Chequebook mock
	chequebookOpts := []chequebookmock.Option{
		chequebookmock.WithChequebookBalanceFunc(func(context.Context) (*big.Int, error) {
			return big.NewInt(2000), nil
		}),
		chequebookmock.WithChequebookAvailableBalanceFunc(func(context.Context) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		chequebookmock.WithChequebookAddressFunc(func() common.Address {
			return common.HexToAddress("0x1234")
		}),
		chequebookmock.WithChequebookDepositFunc(func(ctx context.Context, amount *big.Int) (common.Hash, error) {
			if amount.Cmp(big.NewInt(100)) == 0 {
				return common.HexToHash("0x111111"), nil
			}
			return common.Hash{}, errors.New("deposit failed")
		}),
		chequebookmock.WithChequebookWithdrawFunc(func(ctx context.Context, amount *big.Int) (common.Hash, error) {
			if amount.Cmp(big.NewInt(50)) == 0 {
				return common.HexToHash("0x222222"), nil
			}
			return common.Hash{}, errors.New("withdraw failed")
		}),
	}

	// Define Swap mock
	swapOpts := []swapmock.Option{
		swapmock.WithLastSentChequeFunc(func(swarm.Address) (*chequebook.SignedCheque, error) {
			return &chequebook.SignedCheque{
				Cheque: chequebook.Cheque{
					Chequebook:       common.HexToAddress("0x1234"),
					Beneficiary:      common.HexToAddress("0x5678"),
					CumulativePayout: big.NewInt(100),
				},
			}, nil
		}),
		swapmock.WithLastReceivedChequeFunc(func(swarm.Address) (*chequebook.SignedCheque, error) {
			return &chequebook.SignedCheque{
				Cheque: chequebook.Cheque{
					Chequebook:       common.HexToAddress("0x5678"),
					Beneficiary:      common.HexToAddress("0x1234"),
					CumulativePayout: big.NewInt(50),
				},
			}, nil
		}),
		swapmock.WithLastSentChequesFunc(func() (map[string]*chequebook.SignedCheque, error) {
			return map[string]*chequebook.SignedCheque{
				"ca11957861758380": {
					Cheque: chequebook.Cheque{
						Chequebook:       common.HexToAddress("0x1234"),
						Beneficiary:      common.HexToAddress("0x5678"),
						CumulativePayout: big.NewInt(100),
					},
				},
			}, nil
		}),
		swapmock.WithLastReceivedChequesFunc(func() (map[string]*chequebook.SignedCheque, error) {
			return map[string]*chequebook.SignedCheque{
				"ca11957861758380": {
					Cheque: chequebook.Cheque{
						Chequebook:       common.HexToAddress("0x5678"),
						Beneficiary:      common.HexToAddress("0x1234"),
						CumulativePayout: big.NewInt(50),
					},
				},
			}, nil
		}),
		swapmock.WithCashoutStatusFunc(func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
			return &chequebook.CashoutStatus{
				UncashedAmount: big.NewInt(10),
			}, nil
		}),
		swapmock.WithCashChequeFunc(func(ctx context.Context, peer swarm.Address) (common.Hash, error) {
			return common.HexToHash("0x333333"), nil
		}),
		swapmock.WithSettlementsSentFunc(func() (map[string]*big.Int, error) {
			return map[string]*big.Int{
				"ca11957861758381": big.NewInt(50),
			}, nil
		}),
		swapmock.WithSettlementsRecvFunc(func() (map[string]*big.Int, error) {
			return map[string]*big.Int{
				"ca11957861758381": big.NewInt(10),
			}, nil
		}),
	}

	// Define ERC20 mock
	erc20Opts := []erc20mock.Option{
		erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		erc20mock.WithTransferFunc(func(ctx context.Context, address common.Address, amount *big.Int) (common.Hash, error) {
			return common.HexToHash("0x444444"), nil
		}),
	}

	// Define Transaction mock
	transactionOpts := []transactionmock.Option{
		transactionmock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (common.Hash, error) {
			return common.HexToHash("0x555555"), nil
		}),
		transactionmock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
			return []common.Hash{common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")}, nil
		}),
		transactionmock.WithStoredTransactionFunc(func(hash common.Hash) (*transaction.StoredTransaction, error) {
			if hash == common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
				return &transaction.StoredTransaction{
					To:       &common.Address{},
					Data:     []byte{},
					GasPrice: big.NewInt(100),
					GasLimit: 21000,
					Value:    big.NewInt(0),
					Nonce:    0,
				}, nil
			}
			return nil, errors.New("transaction not found")
		}),
		transactionmock.WithResendTransactionFunc(func(ctx context.Context, hash common.Hash) error {
			if hash == common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
				return nil
			}
			return errors.New("resend transaction failed")
		}),
		transactionmock.WithCancelTransactionFunc(func(ctx context.Context, hash common.Hash) (common.Hash, error) {
			if hash == common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
				return common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"), nil
			}
			return common.Hash{}, errors.New("cancel transaction failed")
		}),
	}

	// Define Backend mock
	backendOpts := []backendmock.Option{
		backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, blockNumber *big.Int) (*big.Int, error) {
			return big.NewInt(500), nil
		}),
	}
	// Define Steward mock
	stewardMock := &mockSteward{}
	stewardMock.isRetrievableFunc = func(ctx context.Context, addr swarm.Address) (bool, error) {
		return true, nil
	}

	// Define Storer mock
	storerMock := mockstorer.New()
	storerMock.NewSession() // Create session with ID 1
	putter, _ := storerMock.Upload(context.Background(), true, 0)
	putter.Done(swarm.MustParseHexAddress("ca11957861758380"))

	// Define Staking mock
	stakingContractMock := stakingmock.New(
		stakingmock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
			if stakedAmount.Cmp(big.NewInt(100)) == 0 {
				return common.HexToHash("0x666666"), nil
			}
			return common.Hash{}, errors.New("deposit stake failed")
		}),
		stakingmock.WithWithdrawStake(func(ctx context.Context) (common.Hash, error) {
			return common.HexToHash("0x777777"), nil
		}),
		stakingmock.WithMigrateStake(func(ctx context.Context) (common.Hash, error) {
			return common.HexToHash("0x888888"), nil
		}),
	)

	// Define Postage Contract mock
	postageContractMock := postagecontractmock.New(
		postagecontractmock.WithCreateBatchFunc(func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (common.Hash, []byte, error) {
			if initialBalance.Cmp(big.NewInt(1000)) != 0 || depth != 17 || !immutable || label != "test-label" {
				// Debug logging
				fmt.Printf("CreateBatch mismatch: balance=%s depth=%d immutable=%v label=%s\n", initialBalance, depth, immutable, label)
				return common.Hash{}, nil, errors.New("create batch failed")
			}
			return common.HexToHash("0x999999"), []byte("batch-id"), nil
		}),
		postagecontractmock.WithTopUpBatchFunc(func(ctx context.Context, batchID []byte, amount *big.Int) (common.Hash, error) {
			if string(batchID) != "batch-id" || amount.Cmp(big.NewInt(500)) != 0 {
				fmt.Printf("TopUpBatch mismatch: batchID=%x (%s) amount=%s\n", batchID, string(batchID), amount)
				return common.Hash{}, errors.New("topup batch failed")
			}
			return common.HexToHash("0xaaaaaa"), nil
		}),
		postagecontractmock.WithDiluteBatchFunc(func(ctx context.Context, batchID []byte, depth uint8) (common.Hash, error) {
			if string(batchID) != "batch-id" || depth != 18 {
				fmt.Printf("DiluteBatch mismatch: batchID=%x (%s) depth=%d\n", batchID, string(batchID), depth)
				return common.Hash{}, errors.New("dilute batch failed")
			}
			return common.HexToHash("0xbbbbbb"), nil
		}),
	)

	client, _, _, _ := newTestServer(t, testServerOptions{
		Overlay:         overlay,
		PublicKey:       privateKey.PublicKey,
		PSSPublicKey:    pssPrivateKey.PublicKey,
		P2P:             p2pService,
		AccountingOpts:  accountingOpts,
		SwapOpts:        swapOpts,
		ChequebookOpts:  chequebookOpts,
		Steward:         stewardMock,
		Storer:          storerMock,
		Erc20Opts:       erc20Opts,
		BackendOpts:     backendOpts,
		WhitelistedAddr: "0x0000000000000000000000000000000000001234",
		StakingContract: stakingContractMock,
		PostageContract: postageContractMock,
		TransactionOpts: transactionOpts,
	})

	t.Run("bee_apiVersion", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_apiVersion",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  string `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result != "v1" {
			t.Errorf("got result %s want v1", resp.Result)
		}
		if resp.ID != 1 {
			t.Errorf("got id %d want 1", resp.ID)
		}
	})

	t.Run("bee_nodeInfo", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_nodeInfo",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				BeeMode           string `json:"beeMode"`
				ChequebookEnabled bool   `json:"chequebookEnabled"`
				SwapEnabled       bool   `json:"swapEnabled"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.BeeMode != "full" {
			t.Errorf("got beeMode %s want full", resp.Result.BeeMode)
		}
		if resp.Result.ChequebookEnabled != true {
			t.Errorf("got chequebookEnabled %v want true", resp.Result.ChequebookEnabled)
		}
		if resp.Result.SwapEnabled != true {
			t.Errorf("got swapEnabled %v want true", resp.Result.SwapEnabled)
		}
	})

	t.Run("bee_addresses", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_addresses",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Overlay string `json:"overlay"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.Overlay == "" {
			t.Error("got empty overlay address")
		}
	})

	t.Run("bee_chainState", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chainState",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				ChainTip uint64 `json:"chainTip"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_peers", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_peers",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Peers []any `json:"peers"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if len(resp.Result.Peers) == 0 {
			t.Error("got empty peers list")
		}
	})

	t.Run("bee_blocklistedPeers", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_blocklistedPeers",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Peers []any `json:"peers"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if len(resp.Result.Peers) == 0 {
			t.Error("got empty blocklisted peers list")
		}
	})

	t.Run("bee_topology", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_topology",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				BaseAddr string `json:"baseAddr"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		// BaseAddr should be empty string as per default mock, just checking call success
	})

	t.Run("bee_balances", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_balances",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Balances []struct {
					Peer    string         `json:"peer"`
					Balance *bigint.BigInt `json:"balance"`
				} `json:"balances"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if len(resp.Result.Balances) == 0 {
			t.Error("got empty balances list")
		}
		if resp.Result.Balances[0].Balance.Int64() != 100 {
			t.Errorf("got balance %d want 100", resp.Result.Balances[0].Balance.Int64())
		}
	})

	t.Run("bee_settlements", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_settlements",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TotalReceived *bigint.BigInt `json:"totalReceived"`
				TotalSent     *bigint.BigInt `json:"totalSent"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.TotalReceived.Int64() != 10 {
			t.Errorf("got totalReceived %d want 10", resp.Result.TotalReceived.Int64())
		}
		if resp.Result.TotalSent.Int64() != 50 {
			t.Errorf("got totalSent %d want 50", resp.Result.TotalSent.Int64())
		}
	})

	t.Run("bee_chequebookBalance", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookBalance",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TotalBalance     *bigint.BigInt `json:"totalBalance"`
				AvailableBalance *bigint.BigInt `json:"availableBalance"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TotalBalance == nil {
			t.Errorf("got nil TotalBalance")
		} else if resp.Result.TotalBalance.Int64() != 2000 {
			t.Errorf("got totalBalance %d want 2000", resp.Result.TotalBalance.Int64())
		}
		if resp.Result.AvailableBalance == nil {
			t.Errorf("got nil AvailableBalance")
		} else if resp.Result.AvailableBalance.Int64() != 1000 {
			t.Errorf("got availableBalance %d want 1000", resp.Result.AvailableBalance.Int64())
		}
	})

	t.Run("bee_chequebookAddress", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookAddress",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Address string `json:"chequebookAddress"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		// Expect default mock address for chequebook, usually factory address or similar
		if resp.Result.Address == "" {
			t.Error("got empty chequebook address")
		}
	})
	t.Run("bee_chequebookLastPeer", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookLastPeer",
			Params:  []any{"ca11957861758381"},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Peer         string `json:"peer"`
				LastReceived any    `json:"lastreceived"`
				LastSent     any    `json:"lastsent"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_chequebookLastCheques", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookLastCheques",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				LastCheques []any `json:"lastcheques"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_chequebookCashoutStatus", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookCashoutStatus",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758381")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Peer           string         `json:"peer"`
				UncashedAmount *bigint.BigInt `json:"uncashedAmount"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_isRetrievable", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_isRetrievable",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758381")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  bool   `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if !resp.Result {
			t.Error("got isRetrievable false want true")
		}
	})

	t.Run("bee_disconnect", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_disconnect",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758383")},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
	})

	t.Run("bee_chequebookDeposit", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookDeposit",
			Params:  []any{"100"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000111111" {
			t.Errorf("got tx hash %s want 0x...111111", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_chequebookWithdraw", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookWithdraw",
			Params:  []any{"50"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000222222" {
			t.Errorf("got tx hash %s want 0x...222222", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_chequebookCashout", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_chequebookCashout",
			Params:  []any{"ca11957861758380"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000333333" {
			t.Errorf("got tx hash %s want 0x...333333", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_depositStake", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_depositStake",
			Params:  []any{"100"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000666666" {
			t.Errorf("got tx hash %s want 0x...666666", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_withdrawStake", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_withdrawStake",
			Params:  []any{},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000777777" {
			t.Errorf("got tx hash %s want 0x...777777", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_migrateStake", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_migrateStake",
			Params:  []any{},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000888888" {
			t.Errorf("got tx hash %s want 0x...888888", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_createBatch", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_createBatch",
			Params:  []any{"1000", 17, true, "test-label"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				BatchID         string `json:"batchID"`
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000999999" {
			t.Errorf("got tx hash %s want 0x...999999", resp.Result.TransactionHash)
		}
		// "batch-id" in hex is "62617463682d6964"
		if resp.Result.BatchID != "62617463682d6964" {
			t.Errorf("got batch id %s want 62617463682d6964", resp.Result.BatchID)
		}
	})

	t.Run("bee_topupBatch", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_topupBatch",
			Params:  []any{"62617463682d6964", "500"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				BatchID         string `json:"batchID"`
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000aaaaaa" {
			t.Errorf("got tx hash %s want 0x...aaaaaa", resp.Result.TransactionHash)
		}
		if resp.Result.BatchID != "62617463682d6964" {
			t.Errorf("got batch id %s want 62617463682d6964", resp.Result.BatchID)
		}
	})

	t.Run("bee_diluteBatch", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_diluteBatch",
			Params:  []any{"62617463682d6964", 18},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				BatchID         string `json:"batchID"`
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000bbbbbb" {
			t.Errorf("got tx hash %s want 0x...bbbbbb", resp.Result.TransactionHash)
		}
		if resp.Result.BatchID != "62617463682d6964" {
			t.Errorf("got batch id %s want 62617463682d6964", resp.Result.BatchID)
		}
	})

	t.Run("bee_transactions", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_transactions",
			Params:  []any{},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				PendingTransactions []struct {
					TransactionHash string `json:"transactionHash"`
				} `json:"pendingTransactions"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if len(resp.Result.PendingTransactions) != 1 {
			t.Errorf("got %d pending transactions want 1", len(resp.Result.PendingTransactions))
		}
		if resp.Result.PendingTransactions[0].TransactionHash != "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
			t.Errorf("got tx hash %s want 0xcccc...", resp.Result.PendingTransactions[0].TransactionHash)
		}
	})

	t.Run("bee_transactionDetail", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_transactionDetail",
			Params:  []any{"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
			t.Errorf("got tx hash %s want 0xcccc...", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_resendTransaction", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_resendTransaction",
			Params:  []any{"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
			t.Errorf("got tx hash %s want 0xcccc...", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_cancelTransaction", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_cancelTransaction",
			Params:  []any{"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
			ID:      1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" {
			t.Errorf("got tx hash %s want 0xdddd...", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_walletWithdraw", func(t *testing.T) {
		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_walletWithdraw",
			// coin, address, amount
			Params: []any{"BZZ", "0x0000000000000000000000000000000000001234", "10"},
			ID:     1,
		}

		body, _ := json.Marshal(req)
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				TransactionHash string `json:"transactionHash"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000444444" {
			t.Errorf("got tx hash %s want 0x...444444", resp.Result.TransactionHash)
		}
	})

	t.Run("bee_tag", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_tag",
			Params:  []any{1},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Uid       uint64    `json:"uid"`
				StartedAt time.Time `json:"startedAt"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.Uid != 1 {
			t.Errorf("got tag uid %d want 1", resp.Result.Uid)
		}
	})

	t.Run("bee_listTags", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_listTags",
			Params:  []any{0, 10},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Tags []struct {
					Uid uint64 `json:"uid"`
				} `json:"tags"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if len(resp.Result.Tags) == 0 {
			t.Error("got empty tags list")
		}
		if resp.Result.Tags[0].Uid != 1 {
			t.Errorf("got tag uid %d want 1", resp.Result.Tags[0].Uid)
		}
	})

	t.Run("bee_pin", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_pin",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758380")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Reference string `json:"reference"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.Reference != "ca11957861758380" {
			t.Errorf("got reference %s want ca11957861758380", resp.Result.Reference)
		}
	})

	t.Run("bee_listPins", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_listPins",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				References []string `json:"references"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if len(resp.Result.References) == 0 {
			t.Error("got empty pins list")
		}
		if resp.Result.References[0] != "ca11957861758380" {
			t.Errorf("got reference %s want ca11957861758380", resp.Result.References[0])
		}
	})

	t.Run("bee_createTag", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_createTag",
			Params:  []any{},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Uid uint64 `json:"uid"`
			} `json:"result"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Result.Uid == 0 {
			t.Error("got empty tag uid")
		}
	})

	t.Run("bee_deleteTag", func(t *testing.T) {

		// Create a tag first to delete
		tag, _ := storerMock.NewSession()

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_deleteTag",
			Params:  []any{tag.TagID},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_doneSplit", func(t *testing.T) {

		// Create a tag first
		tag, _ := storerMock.NewSession()

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_doneSplit",
			Params:  []any{tag.TagID, swarm.MustParseHexAddress("ca11957861758380")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_pinRootHash", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_pinRootHash",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758381")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_unpinRootHash", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_unpinRootHash",
			Params:  []any{swarm.MustParseHexAddress("ca11957861758380")},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			ID      int    `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
	})

	t.Run("bee_connect", func(t *testing.T) {

		req := struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			ID      int    `json:"id"`
		}{
			JSONRPC: "2.0",
			Method:  "bee_connect",
			Params:  []any{"/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS"},
			ID:      1,
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Address string `json:"address"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID int `json:"id"`
		}

		jsonhttptest.Request(t, client, http.MethodPost, "/rpc", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader(body)),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		if resp.JSONRPC != "2.0" {
			t.Errorf("got jsonrpc %s want 2.0", resp.JSONRPC)
		}
		if resp.Error != nil {
			t.Errorf("got rpc error: %d %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Result.Address != "ca11957861758383" {
			t.Errorf("got address %s want ca11957861758383", resp.Result.Address)
		}
	})
}

type mockSteward struct {
	reuploadFunc      func(context.Context, swarm.Address) error
	isRetrievableFunc func(context.Context, swarm.Address) (bool, error)
}

func (m *mockSteward) Reupload(ctx context.Context, addr swarm.Address, _ postage.Stamper) error {
	if m.reuploadFunc != nil {
		return m.reuploadFunc(ctx, addr)
	}
	return nil
}

func (m *mockSteward) IsRetrievable(ctx context.Context, addr swarm.Address) (bool, error) {
	if m.isRetrievableFunc != nil {
		return m.isRetrievableFunc(ctx, addr)
	}
	return false, nil
}
