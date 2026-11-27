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
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	chequestoremock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequestore/mock"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	chequebookABI          = abiutil.MustParseABI(sw3abi.ERC20SimpleSwapABIv0_6_9)
	chequeCashedEventType  = chequebookABI.Events["ChequeCashed"]
	chequeBouncedEventType = chequebookABI.Events["ChequeBounced"]
)

func TestCashout(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      common.HexToAddress("aaaa"),
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return txHash, false, nil
			}),
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}

				logData, err := chequeCashedEventType.Inputs.NonIndexed().Pack(totalPayout, cumulativePayout, big.NewInt(0))
				if err != nil {
					t.Fatal(err)
				}

				return &types.Receipt{
					TxHash: txHash,
					Status: types.ReceiptStatusSuccessful,
					Logs: []*types.Log{
						{
							Address: chequebookAddress,
							Topics: []common.Hash{
								chequeCashedEventType.ID,
								common.BytesToHash(cheque.Beneficiary.Bytes()),
								common.BytesToHash(recipientAddress.Bytes()),
								common.BytesToHash(cheque.Beneficiary.Bytes()),
							},
							Data: logData,
						},
					},
				}, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
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
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      common.HexToAddress("aaaa"),
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return txHash, false, nil
			}),
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}

				chequeCashedLogData, err := chequeCashedEventType.Inputs.NonIndexed().Pack(totalPayout, cumulativePayout, big.NewInt(0))
				if err != nil {
					t.Fatal(err)
				}

				return &types.Receipt{
					TxHash: txHash,
					Status: types.ReceiptStatusSuccessful,
					Logs: []*types.Log{
						{
							Address: chequebookAddress,
							Topics: []common.Hash{
								chequeCashedEventType.ID,
								common.BytesToHash(cheque.Beneficiary.Bytes()),
								common.BytesToHash(recipientAddress.Bytes()),
								common.BytesToHash(cheque.Beneficiary.Bytes()),
							},
							Data: chequeCashedLogData,
						},
						{
							Address: chequebookAddress,
							Topics:  []common.Hash{chequeBouncedEventType.ID},
						},
					},
				}, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
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
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	cumulativePayout := big.NewInt(500)
	onChainPaidOut := big.NewInt(100)
	beneficiary := common.HexToAddress("aaaa")

	cheque := &chequebook.SignedCheque{
		Beneficiary:      beneficiary,
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return txHash, false, nil
			}),
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithABICall(&chequebookABI, chequebookAddress, onChainPaidOut.FillBytes(make([]byte, 32)), "paidOut", beneficiary),
			transactionmock.WithTransactionReceiptFunc(func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != txHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", txHash, hash)
				}
				return &types.Receipt{
					TxHash: txHash,
					Status: types.ReceiptStatusFailed,
				}, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
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
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      common.HexToAddress("aaaa"),
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return txHash, true, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
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

func TestCashoutStatusPendingReplacement(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	replacement := common.HexToHash("eeee")
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      common.HexToAddress("aaaa"),
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return replacement, true, nil
			}),
			transactionmock.WithTransactionReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
				t.Fatal("receipt requested for a pending cashout")
				return nil, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(common.Address) (*chequebook.SignedCheque, error) {
				return cheque, nil
			}),
		),
	)

	if _, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress); err != nil {
		t.Fatal(err)
	}
	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
	if status.Last == nil || status.Last.TxHash != replacement || status.Last.Result != nil {
		t.Fatalf("status = %+v", status.Last)
	}
}

func TestCashoutStatusResent(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("abcd")
	recipientAddress := common.HexToAddress("efff")
	txHash := common.HexToHash("dddd")
	minedHash := common.HexToHash("eeee")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(500)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      common.HexToAddress("aaaa"),
		CumulativePayout: cumulativePayout,
		Chequebook:       chequebookAddress,
		Signature:        []byte{},
	}

	store := storemock.NewStateStore()
	cashoutService := chequebook.NewCashoutService(
		store,
		transactionmock.New(
			transactionmock.WithKnownTransactionFunc(func(_ context.Context, hash common.Hash) (common.Hash, bool, error) {
				if hash != txHash {
					t.Fatalf("known transaction for %v, want %v", hash, txHash)
				}
				return minedHash, false, nil
			}),
			transactionmock.WithABISend(&chequebookABI, txHash, chequebookAddress, big.NewInt(0), "cashChequeBeneficiary", recipientAddress, cheque.CumulativePayout, cheque.Signature),
			transactionmock.WithTransactionReceiptFunc(func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
				if hash != minedHash {
					t.Fatalf("fetching receipt for transaction. wanted %v, got %v", minedHash, hash)
				}
				logData, err := chequeCashedEventType.Inputs.NonIndexed().Pack(totalPayout, cumulativePayout, big.NewInt(0))
				if err != nil {
					t.Fatal(err)
				}
				return &types.Receipt{
					TxHash: minedHash,
					Status: types.ReceiptStatusSuccessful,
					Logs: []*types.Log{{
						Address: chequebookAddress,
						Topics: []common.Hash{
							chequeCashedEventType.ID,
							common.BytesToHash(cheque.Beneficiary.Bytes()),
							common.BytesToHash(recipientAddress.Bytes()),
							common.BytesToHash(cheque.Beneficiary.Bytes()),
						},
						Data: logData,
					}},
				}, nil
			}),
		),
		chequestoremock.NewChequeStore(
			chequestoremock.WithLastReceivedChequeFunc(func(c common.Address) (*chequebook.SignedCheque, error) {
				return cheque, nil
			}),
		),
	)

	if _, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress); err != nil {
		t.Fatal(err)
	}

	status, err := cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
	if status.Last == nil || status.Last.TxHash != minedHash {
		t.Fatalf("tx hash = %v", status.Last)
	}
	if status.Last.Result == nil || status.Last.Result.CumulativePayout.Cmp(cumulativePayout) != 0 {
		t.Fatalf("result = %v", status.Last.Result)
	}
}

func verifyStatus(t *testing.T, status *chequebook.CashoutStatus, expected chequebook.CashoutStatus) {
	t.Helper()

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
