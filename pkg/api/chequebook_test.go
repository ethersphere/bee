// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestChequebookBalance(t *testing.T) {
	t.Parallel()

	returnedBalance := big.NewInt(9000)
	returnedAvailableBalance := big.NewInt(1000)

	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return returnedBalance, nil
	}

	chequebookAvailableBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return returnedAvailableBalance, nil
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{
			mock.WithChequebookBalanceFunc(chequebookBalanceFunc),
			mock.WithChequebookAvailableBalanceFunc(chequebookAvailableBalanceFunc),
		},
	})

	expected := &api.ChequebookBalanceResponse{
		TotalBalance:     bigint.Wrap(returnedBalance),
		AvailableBalance: bigint.Wrap(returnedAvailableBalance),
	}

	var got *api.ChequebookBalanceResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/balance", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got balance: %+v, expected: %+v", got, expected)
	}
}

func TestChequebookBalanceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("New errors")
	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return big.NewInt(0), wantErr
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookBalanceFunc(chequebookBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/balance", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrChequebookBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestChequebookAvailableBalanceError(t *testing.T) {
	t.Parallel()

	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return big.NewInt(0), nil
	}

	chequebookAvailableBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return nil, errors.New("New errors")
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{
			mock.WithChequebookBalanceFunc(chequebookBalanceFunc),
			mock.WithChequebookAvailableBalanceFunc(chequebookAvailableBalanceFunc),
		},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/balance", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrChequebookBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestChequebookAddress(t *testing.T) {
	t.Parallel()

	chequebookAddressFunc := func() common.Address {
		return common.HexToAddress("0xfffff")
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookAddressFunc(chequebookAddressFunc)},
	})

	address := common.HexToAddress("0xfffff")

	expected := &api.ChequebookAddressResponse{
		Address: address.String(),
	}

	var got *api.ChequebookAddressResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/address", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got address: %+v, expected: %+v", got, expected)
	}
}

func TestChequebookWithdraw(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0xfffff")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		chequebookWithdrawFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if amount.Cmp(big.NewInt(500)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookWithdrawFunc(chequebookWithdrawFunc)},
		})

		expected := &api.ChequebookTxResponse{TransactionHash: txHash}

		var got *api.ChequebookTxResponse
		jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/withdraw?amount=500", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})

	t.Run("custom gas", func(t *testing.T) {
		t.Parallel()

		chequebookWithdrawFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10)) != 0 {
				return common.Hash{}, errors.New("wrong gas price")
			}
			if amount.Cmp(big.NewInt(500)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookWithdrawFunc(chequebookWithdrawFunc)},
		})

		expected := &api.ChequebookTxResponse{TransactionHash: txHash}

		var got *api.ChequebookTxResponse
		jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/withdraw?amount=500", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.GasPriceHeader, "10"),
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})
}

func TestChequebookDeposit(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0xfffff")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		chequebookDepositFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if amount.Cmp(big.NewInt(700)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookDepositFunc(chequebookDepositFunc)},
		})

		expected := &api.ChequebookTxResponse{TransactionHash: txHash}

		var got *api.ChequebookTxResponse
		jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/deposit?amount=700", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})

	t.Run("custom gas", func(t *testing.T) {
		t.Parallel()

		chequebookDepositFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10)) != 0 {
				return common.Hash{}, errors.New("wrong gas price")
			}

			if amount.Cmp(big.NewInt(700)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookDepositFunc(chequebookDepositFunc)},
		})

		expected := &api.ChequebookTxResponse{TransactionHash: txHash}

		var got *api.ChequebookTxResponse
		jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/deposit?amount=700", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.GasPriceHeader, "10"),
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})
}

func TestChequebookLastCheques(t *testing.T) {
	t.Parallel()

	addr1 := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	addr2 := swarm.MustParseHexAddress("2000000000000000000000000000000000000000000000000000000000000000")
	addr3 := swarm.MustParseHexAddress("3000000000000000000000000000000000000000000000000000000000000000")
	addr4 := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")
	addr5 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	beneficiary := common.HexToAddress("0xfff5")
	beneficiary1 := common.HexToAddress("0xfff0")
	beneficiary2 := common.HexToAddress("0xfff1")
	beneficiary3 := common.HexToAddress("0xfff2")
	cumulativePayout1 := big.NewInt(700)
	cumulativePayout2 := big.NewInt(900)
	cumulativePayout3 := big.NewInt(600)
	cumulativePayout4 := big.NewInt(550)
	cumulativePayout5 := big.NewInt(400)
	cumulativePayout6 := big.NewInt(720)
	chequebookAddress1 := common.HexToAddress("0xeee1")
	chequebookAddress2 := common.HexToAddress("0xeee2")
	chequebookAddress3 := common.HexToAddress("0xeee3")
	chequebookAddress4 := common.HexToAddress("0xeee4")
	chequebookAddress5 := common.HexToAddress("0xeee5")

	lastSentChequesFunc := func() (map[string]*chequebook.SignedCheque, error) {
		lastSentCheques := make(map[string]*chequebook.SignedCheque, 3)
		sig := make([]byte, 65)
		lastSentCheques[addr1.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary1,
				CumulativePayout: cumulativePayout1,
				Chequebook:       chequebookAddress1,
			},
			Signature: sig,
		}

		lastSentCheques[addr2.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary2,
				CumulativePayout: cumulativePayout2,
				Chequebook:       chequebookAddress2,
			},
			Signature: sig,
		}

		lastSentCheques[addr3.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary3,
				CumulativePayout: cumulativePayout3,
				Chequebook:       chequebookAddress3,
			},
			Signature: sig,
		}
		return lastSentCheques, nil
	}

	lastReceivedChequesFunc := func() (map[string]*chequebook.SignedCheque, error) {
		lastReceivedCheques := make(map[string]*chequebook.SignedCheque, 3)
		sig := make([]byte, 65)

		lastReceivedCheques[addr1.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary,
				CumulativePayout: cumulativePayout4,
				Chequebook:       chequebookAddress1,
			},
			Signature: sig,
		}

		lastReceivedCheques[addr4.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary,
				CumulativePayout: cumulativePayout5,
				Chequebook:       chequebookAddress4,
			},
			Signature: sig,
		}

		lastReceivedCheques[addr5.String()] = &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary,
				CumulativePayout: cumulativePayout6,
				Chequebook:       chequebookAddress5,
			},
			Signature: sig,
		}

		return lastReceivedCheques, nil
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithLastReceivedChequesFunc(lastReceivedChequesFunc), swapmock.WithLastSentChequesFunc(lastSentChequesFunc)},
	})

	lastchequesexpected := []api.ChequebookLastChequesPeerResponse{
		{
			Peer: addr1.String(),
			LastReceived: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress1.String(),
				Payout:      bigint.Wrap(cumulativePayout4),
			},
			LastSent: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary1.String(),
				Chequebook:  chequebookAddress1.String(),
				Payout:      bigint.Wrap(cumulativePayout1),
			},
		},
		{
			Peer:         addr2.String(),
			LastReceived: nil,
			LastSent: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary2.String(),
				Chequebook:  chequebookAddress2.String(),
				Payout:      bigint.Wrap(cumulativePayout2),
			},
		},
		{
			Peer:         addr3.String(),
			LastReceived: nil,
			LastSent: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary3.String(),
				Chequebook:  chequebookAddress3.String(),
				Payout:      bigint.Wrap(cumulativePayout3),
			},
		},
		{
			Peer: addr4.String(),
			LastReceived: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress4.String(),
				Payout:      bigint.Wrap(cumulativePayout5),
			},
			LastSent: nil,
		},
		{
			Peer: addr5.String(),
			LastReceived: &api.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress5.String(),
				Payout:      bigint.Wrap(cumulativePayout6),
			},
			LastSent: nil,
		},
	}

	expected := &api.ChequebookLastChequesResponse{
		LastCheques: lastchequesexpected,
	}

	// We expect a list of items unordered by peer:
	var got *api.ChequebookLastChequesResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/cheque", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !LastChequesEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}
}

func TestChequebookLastChequesPeer(t *testing.T) {
	t.Parallel()

	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	beneficiary0 := common.HexToAddress("0xfff5")
	beneficiary1 := common.HexToAddress("0xfff0")
	cumulativePayout1 := big.NewInt(700)
	cumulativePayout2 := big.NewInt(900)
	chequebookAddress := common.HexToAddress("0xeee1")
	sig := make([]byte, 65)

	lastSentChequeFunc := func(swarm.Address) (*chequebook.SignedCheque, error) {
		sig := make([]byte, 65)

		lastSentCheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary1,
				CumulativePayout: cumulativePayout1,
				Chequebook:       chequebookAddress,
			},
			Signature: sig,
		}

		return lastSentCheque, nil
	}

	lastReceivedChequeFunc := func(swarm.Address) (*chequebook.SignedCheque, error) {
		lastReceivedCheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary0,
				CumulativePayout: cumulativePayout2,
				Chequebook:       chequebookAddress,
			},
			Signature: sig,
		}

		return lastReceivedCheque, nil
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithLastReceivedChequeFunc(lastReceivedChequeFunc), swapmock.WithLastSentChequeFunc(lastSentChequeFunc)},
	})

	expected := &api.ChequebookLastChequesPeerResponse{
		Peer: addr.String(),
		LastReceived: &api.ChequebookLastChequePeerResponse{
			Beneficiary: beneficiary0.String(),
			Chequebook:  chequebookAddress.String(),
			Payout:      bigint.Wrap(cumulativePayout2),
		},
		LastSent: &api.ChequebookLastChequePeerResponse{
			Beneficiary: beneficiary1.String(),
			Chequebook:  chequebookAddress.String(),
			Payout:      bigint.Wrap(cumulativePayout1),
		},
	}

	var got *api.ChequebookLastChequesPeerResponse
	jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/cheque/"+addr.String(), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}
}

func TestChequebookCashout(t *testing.T) {
	t.Parallel()

	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	deployCashingHash := common.HexToHash("0xffff")

	cashChequeFunc := func(ctx context.Context, peer swarm.Address) (common.Hash, error) {
		return deployCashingHash, nil
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithCashChequeFunc(cashChequeFunc)},
	})

	expected := &api.SwapCashoutResponse{TransactionHash: deployCashingHash.String()}

	var got *api.SwapCashoutResponse
	jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/cashout/"+addr.String(), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}
}

func TestChequebookCashout_CustomGas(t *testing.T) {
	t.Parallel()

	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	deployCashingHash := common.HexToHash("0xffff")

	var price *big.Int
	var limit uint64
	cashChequeFunc := func(ctx context.Context, peer swarm.Address) (common.Hash, error) {
		price = sctx.GetGasPrice(ctx)
		limit = sctx.GetGasLimit(ctx)
		return deployCashingHash, nil
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithCashChequeFunc(cashChequeFunc)},
	})

	expected := &api.SwapCashoutResponse{TransactionHash: deployCashingHash.String()}

	var got *api.SwapCashoutResponse
	jsonhttptest.Request(t, testServer, http.MethodPost, "/chequebook/cashout/"+addr.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader(api.GasPriceHeader, "10000"),
		jsonhttptest.WithRequestHeader(api.GasLimitHeader, "12221"),
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}

	if price.Cmp(big.NewInt(10000)) != 0 {
		t.Fatalf("expected gas price 10000 got %s", price)
	}

	if limit != 12221 {
		t.Fatalf("expected gas limit 12221 got %d", limit)
	}
}

func TestChequebookCashoutStatus(t *testing.T) {
	t.Parallel()

	actionTxHash := common.HexToHash("0xacfe")
	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	beneficiary := common.HexToAddress("0xfff0")
	recipientAddress := common.HexToAddress("efff")
	totalPayout := big.NewInt(100)
	cumulativePayout := big.NewInt(700)
	uncashedAmount := big.NewInt(200)
	chequebookAddress := common.HexToAddress("0xcfec")
	peer := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")

	sig := make([]byte, 65)
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			CumulativePayout: cumulativePayout,
			Chequebook:       chequebookAddress,
		},
		Signature: sig,
	}

	result := &chequebook.CashChequeResult{
		Beneficiary:      cheque.Beneficiary,
		Recipient:        recipientAddress,
		Caller:           cheque.Beneficiary,
		TotalPayout:      totalPayout,
		CumulativePayout: cumulativePayout,
		CallerPayout:     big.NewInt(0),
		Bounced:          false,
	}

	t.Run("with result", func(t *testing.T) {
		t.Parallel()

		cashoutStatusFunc := func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
			status := &chequebook.CashoutStatus{
				Last: &chequebook.LastCashout{
					TxHash:   actionTxHash,
					Cheque:   *cheque,
					Result:   result,
					Reverted: false,
				},
				UncashedAmount: uncashedAmount,
			}
			return status, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &api.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: &actionTxHash,
			Cheque: &api.ChequebookLastChequePeerResponse{
				Chequebook:  chequebookAddress.String(),
				Payout:      bigint.Wrap(cumulativePayout),
				Beneficiary: cheque.Beneficiary.String(),
			},
			Result: &api.SwapCashoutStatusResult{
				Recipient:  recipientAddress,
				LastPayout: bigint.Wrap(totalPayout),
				Bounced:    false,
			},
			UncashedAmount: bigint.Wrap(uncashedAmount),
		}

		var got *api.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,

			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})

	t.Run("without result", func(t *testing.T) {
		t.Parallel()

		cashoutStatusFunc := func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
			status := &chequebook.CashoutStatus{
				Last: &chequebook.LastCashout{
					TxHash:   actionTxHash,
					Cheque:   *cheque,
					Result:   nil,
					Reverted: false,
				},
				UncashedAmount: uncashedAmount,
			}
			return status, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &api.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: &actionTxHash,
			Cheque: &api.ChequebookLastChequePeerResponse{
				Chequebook:  chequebookAddress.String(),
				Payout:      bigint.Wrap(cumulativePayout),
				Beneficiary: cheque.Beneficiary.String(),
			},
			Result:         nil,
			UncashedAmount: bigint.Wrap(uncashedAmount),
		}

		var got *api.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,

			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})

	t.Run("without last", func(t *testing.T) {
		t.Parallel()

		cashoutStatusFunc := func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
			status := &chequebook.CashoutStatus{
				Last:           nil,
				UncashedAmount: uncashedAmount,
			}
			return status, nil
		}

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &api.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: nil,
			Cheque:          nil,
			Result:          nil,
			UncashedAmount:  bigint.Wrap(uncashedAmount),
		}

		var got *api.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,

			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})
}

func Test_chequebookLastPeerHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name string
		peer string
		want jsonhttp.StatusResponse
	}{{
		name: "peer - odd hex string",
		peer: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "peer",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name: "peer - invalid hex character",
		peer: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "peer",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/chequebook/cheque/"+tc.peer, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func LastChequesEqual(a, b *api.ChequebookLastChequesResponse) bool {
	var state bool

	for akeys := range a.LastCheques {
		state = false
		for bkeys := range b.LastCheques {
			if reflect.DeepEqual(a.LastCheques[akeys], b.LastCheques[bkeys]) {
				state = true
				break
			}
		}
		if !state {
			return false
		}
	}

	for bkeys := range b.LastCheques {
		state = false
		for akeys := range a.LastCheques {
			if reflect.DeepEqual(a.LastCheques[akeys], b.LastCheques[bkeys]) {
				state = true
				break
			}
		}
		if !state {
			return false
		}
	}

	return true
}
