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
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	chequebookABI          = transaction.ParseABIUnchecked(sw3abi.ERC20SimpleSwapABIv0_3_1)
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
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
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

	verifyStatus(t, status, chequebook.CashoutStatus{
		Last: &chequebook.LastCashout{
			TxHash: txHash,
			Cheque: *cheque,
			Result: &chequebook.CashChequeResult{
				Beneficiary:      cheque.Beneficiary,
				Recipient:        recipientAddress,
				Caller:           cheque.Beneficiary,
				TotalPayout:      totalPayout,
				CumulativePayout: cumulativePayout,
				CallerPayout:     big.NewInt(0),
				Bounced:          false,
			},
			Reverted: false,
		},
		UncashedAmount: big.NewInt(0),
	})
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
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
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

	verifyStatus(t, status, chequebook.CashoutStatus{
		Last: &chequebook.LastCashout{
			TxHash: txHash,
			Cheque: *cheque,
			Result: &chequebook.CashChequeResult{
				Beneficiary:      cheque.Beneficiary,
				Recipient:        recipientAddress,
				Caller:           cheque.Beneficiary,
				TotalPayout:      totalPayout,
				CumulativePayout: cumulativePayout,
				CallerPayout:     big.NewInt(0),
				Bounced:          true,
			},
			Reverted: false,
		},
		UncashedAmount: big.NewInt(0),
	})
}

func TestCashoutStatusReverted(t *testing.T) {
	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	cumulativePayout := big.NewInt(500)
	onChainPaidOut := big.NewInt(100)
	beneficiary := common.HexToAddress("aaaa")

	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
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
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithABICall(&chequebookABI, chequebookAddress, onChainPaidOut.FillBytes(make([]byte, 32)), "paidOut", beneficiary),
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

	verifyStatus(t, status, chequebook.CashoutStatus{
		Last: &chequebook.LastCashout{
			Reverted: true,
			TxHash:   txHash,
			Cheque:   *cheque,
		},
		UncashedAmount: new(big.Int).Sub(cheque.CumulativePayout, onChainPaidOut),
	})
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
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
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

	verifyStatus(t, status, chequebook.CashoutStatus{
		Last: &chequebook.LastCashout{
			Reverted: false,
			TxHash:   txHash,
			Cheque:   *cheque,
			Result:   nil,
		},
		UncashedAmount: big.NewInt(0),
	})

}

func verifyStatus(t *testing.T, status *chequebook.CashoutStatus, expected chequebook.CashoutStatus) {
	if expected.Last == nil {
		if status.Last != nil {
			t.Fatal("unexpected last cashout")
		}
	} else {
		if status.Last == nil {
			t.Fatal("no last cashout")
		}
		if status.Last.Reverted != expected.Last.Reverted {
			t.Fatalf("wrong reverted value. wanted %v, got %v", expected.Last.Reverted, status.Last.Reverted)
		}
		if status.Last.TxHash != expected.Last.TxHash {
			t.Fatalf("wrong transaction hash. wanted %v, got %v", expected.Last.TxHash, status.Last.TxHash)
		}
		if !status.Last.Cheque.Equal(&expected.Last.Cheque) {
			t.Fatalf("wrong cheque in status. wanted %v, got %v", expected.Last.Cheque, status.Last.Cheque)
		}

		if expected.Last.Result != nil {
			if !expected.Last.Result.Equal(status.Last.Result) {
				t.Fatalf("wrong result. wanted %v, got %v", expected.Last.Result, status.Last.Result)
			}
		}
	}

	if status.UncashedAmount.Cmp(expected.UncashedAmount) != 0 {
		t.Fatalf("wrong uncashed amount. wanted %d, got %d", expected.UncashedAmount, status.UncashedAmount)
	}
}
