// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	chequestoremock "github.com/ethersphere/bee/pkg/settlement/swap/chequestore/mock"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/settlement/swap/transaction/mock"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/sw3-bindings/v3/simpleswapfactory"
)

var (
	chequebookABI          = transaction.ParseABIUnchecked(simpleswapfactory.ERC20SimpleSwapABI)
	chequeCashedEventType  = chequebookABI.Events["ChequeCashed"]
	chequeBouncedEventType = chequebookABI.Events["ChequeBounced"]
)

func TestCashout(t *testing.T) {
	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("aaaa"),
			CumulativePayout: cumulativePayout,
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		backendmock.New(
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
				if hash != txHash {
					t.Fatalf("fetching wrong transaction. wanted %v, got %v", txHash, hash)
				}
				return nil, false, nil
			}),
			backendmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}

				logData, err := chequeCashedEventType.Inputs.NonIndexed().Pack(totalPayout, cumulativePayout, big.NewInt(0))
				if err != nil {
					t.Fatal(err)
				}

				return &types.Receipt{
					Status: types.ReceiptStatusSuccessful,
					Logs: []*types.Log{
						{
							Address: chequebookAddress,
							Topics:  []common.Hash{chequeCashedEventType.ID, cheque.Beneficiary.Hash(), recipientAddress.Hash(), cheque.Beneficiary.Hash()},
							Data:    logData,
						},
					},
				}, nil
			}),
		),
		transactionmock.New(
			transactionmock.WithSendFunc(func(c context.Context, request *transaction.TxRequest) (common.Hash, error) {
				if request.To != nil && *request.To != chequebookAddress {
					t.Fatalf("sending to wrong contract. wanted %x, got %x", chequebookAddress, request.To)
				}
				if request.Value.Cmp(big.NewInt(0)) != 0 {
					t.Fatal("sending ether to chequebook contract")
				}
				return txHash, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
				if c != chequebookAddress {
					t.Fatalf("using wrong chequebook. wanted %v, got %v", chequebookAddress, c)
				}
				return cheque, nil
			}),
		),
	)

	returnedTxHash, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress)
	if err != nil {
		t.Fatal(err)
	}

	if returnedTxHash != txHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}

	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}

	if status.Reverted {
		t.Fatal("reported reverted transaction")
	}

	if status.TxHash != txHash {
		t.Fatalf("wrong transaction hash. wanted %v, got %v", txHash, status.TxHash)
	}

	if !status.Cheque.Equal(cheque) {
		t.Fatalf("wrong cheque in status. wanted %v, got %v", cheque, status.Cheque)
	}

	if status.Result == nil {
		t.Fatal("missing result")
	}

	expectedResult := &chequebook.CashChequeResult{
		Beneficiary:      cheque.Beneficiary,
		Recipient:        recipientAddress,
		Caller:           cheque.Beneficiary,
		TotalPayout:      totalPayout,
		CumulativePayout: cumulativePayout,
		CallerPayout:     big.NewInt(0),
		Bounced:          false,
	}

	if !status.Result.Equal(expectedResult) {
		t.Fatalf("wrong result. wanted %v, got %v", expectedResult, status.Result)
	}
}

func TestCashoutBounced(t *testing.T) {
	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("aaaa"),
			CumulativePayout: cumulativePayout,
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		backendmock.New(
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
				if hash != txHash {
					t.Fatalf("fetching wrong transaction. wanted %v, got %v", txHash, hash)
				}
				return nil, false, nil
			}),
			backendmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}

				chequeCashedLogData, err := chequeCashedEventType.Inputs.NonIndexed().Pack(totalPayout, cumulativePayout, big.NewInt(0))
				if err != nil {
					t.Fatal(err)
				}

				return &types.Receipt{
					Status: types.ReceiptStatusSuccessful,
					Logs: []*types.Log{
						{
							Address: chequebookAddress,
							Topics:  []common.Hash{chequeCashedEventType.ID, cheque.Beneficiary.Hash(), recipientAddress.Hash(), cheque.Beneficiary.Hash()},
							Data:    chequeCashedLogData,
						},
						{
							Address: chequebookAddress,
							Topics:  []common.Hash{chequeBouncedEventType.ID},
						},
					},
				}, nil
			}),
		),
		transactionmock.New(
			transactionmock.WithSendFunc(func(c context.Context, request *transaction.TxRequest) (common.Hash, error) {
				if request.To != nil && *request.To != chequebookAddress {
					t.Fatalf("sending to wrong contract. wanted %x, got %x", chequebookAddress, request.To)
				}
				if request.Value.Cmp(big.NewInt(0)) != 0 {
					t.Fatal("sending ether to chequebook contract")
				}
				return txHash, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
				if c != chequebookAddress {
					t.Fatalf("using wrong chequebook. wanted %v, got %v", chequebookAddress, c)
				}
				return cheque, nil
			}),
		),
	)

	returnedTxHash, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress)
	if err != nil {
		t.Fatal(err)
	}

	if returnedTxHash != txHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}

	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}

	if status.Reverted {
		t.Fatal("reported reverted transaction")
	}

	if status.TxHash != txHash {
		t.Fatalf("wrong transaction hash. wanted %v, got %v", txHash, status.TxHash)
	}

	if !status.Cheque.Equal(cheque) {
		t.Fatalf("wrong cheque in status. wanted %v, got %v", cheque, status.Cheque)
	}

	if status.Result == nil {
		t.Fatal("missing result")
	}

	expectedResult := &chequebook.CashChequeResult{
		Beneficiary:      cheque.Beneficiary,
		Recipient:        recipientAddress,
		Caller:           cheque.Beneficiary,
		TotalPayout:      totalPayout,
		CumulativePayout: cumulativePayout,
		CallerPayout:     big.NewInt(0),
		Bounced:          true,
	}

	if !status.Result.Equal(expectedResult) {
		t.Fatalf("wrong result. wanted %v, got %v", expectedResult, status.Result)
	}
}

func TestCashoutStatusReverted(t *testing.T) {
	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("aaaa"),
			CumulativePayout: cumulativePayout,
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		backendmock.New(
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
				if hash != txHash {
					t.Fatalf("fetching wrong transaction. wanted %v, got %v", txHash, hash)
				}
				return nil, false, nil
			}),
			backendmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}
				return &types.Receipt{
					Status: types.ReceiptStatusFailed,
				}, nil
			}),
		),
		transactionmock.New(
			transactionmock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, error) {
				return txHash, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
				if c != chequebookAddress {
					t.Fatalf("using wrong chequebook. wanted %v, got %v", chequebookAddress, c)
				}
				return cheque, nil
			}),
		),
	)

	returnedTxHash, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress)
	if err != nil {
		t.Fatal(err)
	}

	if returnedTxHash != txHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}

	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}

	if !status.Reverted {
		t.Fatal("did not report failed transaction as reverted")
	}

	if status.TxHash != txHash {
		t.Fatalf("wrong transaction hash. wanted %v, got %v", txHash, status.TxHash)
	}

	if !status.Cheque.Equal(cheque) {
		t.Fatalf("wrong cheque in status. wanted %v, got %v", cheque, status.Cheque)
	}
}

func TestCashoutStatusPending(t *testing.T) {
	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("aaaa"),
			CumulativePayout: cumulativePayout,
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		backendmock.New(
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
				if hash != txHash {
					t.Fatalf("fetching wrong transaction. wanted %v, got %v", txHash, hash)
				}
				return nil, true, nil
			}),
		),
		transactionmock.New(
			transactionmock.WithSendFunc(func(c context.Context, request *transaction.TxRequest) (common.Hash, error) {
				return txHash, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
				if c != chequebookAddress {
					t.Fatalf("using wrong chequebook. wanted %v, got %v", chequebookAddress, c)
				}
				return cheque, nil
			}),
		),
	)

	returnedTxHash, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress)
	if err != nil {
		t.Fatal(err)
	}

	if returnedTxHash != txHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}

	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}

	if status.Reverted {
		t.Fatal("did report pending transaction as reverted")
	}

	if status.TxHash != txHash {
		t.Fatalf("wrong transaction hash. wanted %v, got %v", txHash, status.TxHash)
	}

	if !status.Cheque.Equal(cheque) {
		t.Fatalf("wrong cheque in status. wanted %v, got %v", cheque, status.Cheque)
	}

	if status.Result != nil {
		t.Fatalf("got result for pending cashout: %v", status.Result)
	}
}
