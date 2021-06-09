// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
	swapmock "github.com/ethersphere/bee/pkg/settlement/swap/mock"

	"github.com/ethersphere/bee/pkg/swarm"
)

func TestChequebookBalance(t *testing.T) {
	returnedBalance := big.NewInt(9000)
	returnedAvailableBalance := big.NewInt(1000)

	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return returnedBalance, nil
	}

	chequebookAvailableBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return returnedAvailableBalance, nil
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{
			mock.WithChequebookBalanceFunc(chequebookBalanceFunc),
			mock.WithChequebookAvailableBalanceFunc(chequebookAvailableBalanceFunc),
		},
	})

	expected := &debugapi.ChequebookBalanceResponse{
		TotalBalance:     bigint.Wrap(returnedBalance),
		AvailableBalance: bigint.Wrap(returnedAvailableBalance),
	}

	var got *debugapi.ChequebookBalanceResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got balance: %+v, expected: %+v", got, expected)
	}

}

func TestChequebookBalanceError(t *testing.T) {
	wantErr := errors.New("New errors")
	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return big.NewInt(0), wantErr
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookBalanceFunc(chequebookBalanceFunc)},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrChequebookBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestChequebookAvailableBalanceError(t *testing.T) {
	chequebookBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return big.NewInt(0), nil
	}

	chequebookAvailableBalanceFunc := func(context.Context) (ret *big.Int, err error) {
		return nil, errors.New("New errors")
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{
			mock.WithChequebookBalanceFunc(chequebookBalanceFunc),
			mock.WithChequebookAvailableBalanceFunc(chequebookAvailableBalanceFunc),
		},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/balance", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: debugapi.ErrChequebookBalance,
			Code:    http.StatusInternalServerError,
		}),
	)
}

func TestChequebookAddress(t *testing.T) {
	chequebookAddressFunc := func() common.Address {
		return common.HexToAddress("0xfffff")
	}

	testServer := newTestServer(t, testServerOptions{
		ChequebookOpts: []mock.Option{mock.WithChequebookAddressFunc(chequebookAddressFunc)},
	})

	address := common.HexToAddress("0xfffff")

	expected := &debugapi.ChequebookAddressResponse{
		Address: address.String(),
	}

	var got *debugapi.ChequebookAddressResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/address", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got address: %+v, expected: %+v", got, expected)
	}
}

func TestChequebookWithdraw(t *testing.T) {

	txHash := common.HexToHash("0xfffff")

	t.Run("ok", func(t *testing.T) {
		chequebookWithdrawFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if amount.Cmp(big.NewInt(500)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookWithdrawFunc(chequebookWithdrawFunc)},
		})

		expected := &debugapi.ChequebookTxResponse{TransactionHash: txHash}

		var got *debugapi.ChequebookTxResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/withdraw?amount=500", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})

	t.Run("custom gas", func(t *testing.T) {
		chequebookWithdrawFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10)) != 0 {
				return common.Hash{}, errors.New("wrong gas price")
			}
			if amount.Cmp(big.NewInt(500)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookWithdrawFunc(chequebookWithdrawFunc)},
		})

		expected := &debugapi.ChequebookTxResponse{TransactionHash: txHash}

		var got *debugapi.ChequebookTxResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/withdraw?amount=500", http.StatusOK,
			jsonhttptest.WithRequestHeader("Gas-Price", "10"),
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})
}

func TestChequebookDeposit(t *testing.T) {

	txHash := common.HexToHash("0xfffff")

	t.Run("ok", func(t *testing.T) {
		chequebookDepositFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if amount.Cmp(big.NewInt(700)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookDepositFunc(chequebookDepositFunc)},
		})

		expected := &debugapi.ChequebookTxResponse{TransactionHash: txHash}

		var got *debugapi.ChequebookTxResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/deposit?amount=700", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})

	t.Run("custom gas", func(t *testing.T) {
		chequebookDepositFunc := func(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
			if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10)) != 0 {
				return common.Hash{}, errors.New("wrong gas price")
			}

			if amount.Cmp(big.NewInt(700)) == 0 {
				return txHash, nil
			}
			return common.Hash{}, nil
		}

		testServer := newTestServer(t, testServerOptions{
			ChequebookOpts: []mock.Option{mock.WithChequebookDepositFunc(chequebookDepositFunc)},
		})

		expected := &debugapi.ChequebookTxResponse{TransactionHash: txHash}

		var got *debugapi.ChequebookTxResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/deposit?amount=700", http.StatusOK,
			jsonhttptest.WithRequestHeader("Gas-Price", "10"),
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Errorf("got address: %+v, expected: %+v", got, expected)
		}
	})
}

func TestChequebookLastCheques(t *testing.T) {
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

	testServer := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithLastReceivedChequesFunc(lastReceivedChequesFunc), swapmock.WithLastSentChequesFunc(lastSentChequesFunc)},
	})

	lastchequesexpected := []debugapi.ChequebookLastChequesPeerResponse{
		{
			Peer: addr1.String(),
			LastReceived: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress1.String(),
				Payout:      bigint.Wrap(cumulativePayout4),
			},
			LastSent: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary1.String(),
				Chequebook:  chequebookAddress1.String(),
				Payout:      bigint.Wrap(cumulativePayout1),
			},
		},
		{
			Peer:         addr2.String(),
			LastReceived: nil,
			LastSent: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary2.String(),
				Chequebook:  chequebookAddress2.String(),
				Payout:      bigint.Wrap(cumulativePayout2),
			},
		},
		{
			Peer:         addr3.String(),
			LastReceived: nil,
			LastSent: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary3.String(),
				Chequebook:  chequebookAddress3.String(),
				Payout:      bigint.Wrap(cumulativePayout3),
			},
		},
		{
			Peer: addr4.String(),
			LastReceived: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress4.String(),
				Payout:      bigint.Wrap(cumulativePayout5),
			},
			LastSent: nil,
		},
		{
			Peer: addr5.String(),
			LastReceived: &debugapi.ChequebookLastChequePeerResponse{
				Beneficiary: beneficiary.String(),
				Chequebook:  chequebookAddress5.String(),
				Payout:      bigint.Wrap(cumulativePayout6),
			},
			LastSent: nil,
		},
	}

	expected := &debugapi.ChequebookLastChequesResponse{
		LastCheques: lastchequesexpected,
	}

	// We expect a list of items unordered by peer:
	var got *debugapi.ChequebookLastChequesResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/cheque", http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !LastChequesEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}

}

func TestChequebookLastChequesPeer(t *testing.T) {

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

	testServer := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithLastReceivedChequeFunc(lastReceivedChequeFunc), swapmock.WithLastSentChequeFunc(lastSentChequeFunc)},
	})

	expected := &debugapi.ChequebookLastChequesPeerResponse{
		Peer: addr.String(),
		LastReceived: &debugapi.ChequebookLastChequePeerResponse{
			Beneficiary: beneficiary0.String(),
			Chequebook:  chequebookAddress.String(),
			Payout:      bigint.Wrap(cumulativePayout2),
		},
		LastSent: &debugapi.ChequebookLastChequePeerResponse{
			Beneficiary: beneficiary1.String(),
			Chequebook:  chequebookAddress.String(),
			Payout:      bigint.Wrap(cumulativePayout1),
		},
	}

	var got *debugapi.ChequebookLastChequesPeerResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/cheque/"+addr.String(), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}

}

func TestChequebookCashout(t *testing.T) {

	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	deployCashingHash := common.HexToHash("0xffff")

	cashChequeFunc := func(ctx context.Context, peer swarm.Address) (common.Hash, error) {
		return deployCashingHash, nil
	}

	testServer := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithCashChequeFunc(cashChequeFunc)},
	})

	expected := &debugapi.SwapCashoutResponse{TransactionHash: deployCashingHash.String()}

	var got *debugapi.SwapCashoutResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/cashout/"+addr.String(), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&got),
	)

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
	}
}

func TestChequebookCashout_CustomGas(t *testing.T) {

	addr := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	deployCashingHash := common.HexToHash("0xffff")

	var price *big.Int
	var limit uint64
	cashChequeFunc := func(ctx context.Context, peer swarm.Address) (common.Hash, error) {
		price = sctx.GetGasPrice(ctx)
		limit = sctx.GetGasLimit(ctx)
		return deployCashingHash, nil
	}

	testServer := newTestServer(t, testServerOptions{
		SwapOpts: []swapmock.Option{swapmock.WithCashChequeFunc(cashChequeFunc)},
	})

	expected := &debugapi.SwapCashoutResponse{TransactionHash: deployCashingHash.String()}

	var got *debugapi.SwapCashoutResponse
	jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/chequebook/cashout/"+addr.String(), http.StatusOK,
		jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
		jsonhttptest.WithRequestHeader("Gas-Limit", "12221"),
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

		testServer := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &debugapi.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: &actionTxHash,
			Cheque: &debugapi.ChequebookLastChequePeerResponse{
				Chequebook:  chequebookAddress.String(),
				Payout:      bigint.Wrap(cumulativePayout),
				Beneficiary: cheque.Beneficiary.String(),
			},
			Result: &debugapi.SwapCashoutStatusResult{
				Recipient:  recipientAddress,
				LastPayout: bigint.Wrap(totalPayout),
				Bounced:    false,
			},
			UncashedAmount: bigint.Wrap(uncashedAmount),
		}

		var got *debugapi.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})

	t.Run("without result", func(t *testing.T) {
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

		testServer := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &debugapi.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: &actionTxHash,
			Cheque: &debugapi.ChequebookLastChequePeerResponse{
				Chequebook:  chequebookAddress.String(),
				Payout:      bigint.Wrap(cumulativePayout),
				Beneficiary: cheque.Beneficiary.String(),
			},
			Result:         nil,
			UncashedAmount: bigint.Wrap(uncashedAmount),
		}

		var got *debugapi.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})

	t.Run("without last", func(t *testing.T) {
		cashoutStatusFunc := func(ctx context.Context, peer swarm.Address) (*chequebook.CashoutStatus, error) {
			status := &chequebook.CashoutStatus{
				Last:           nil,
				UncashedAmount: uncashedAmount,
			}
			return status, nil
		}

		testServer := newTestServer(t, testServerOptions{
			SwapOpts: []swapmock.Option{swapmock.WithCashoutStatusFunc(cashoutStatusFunc)},
		})

		expected := &debugapi.SwapCashoutStatusResponse{
			Peer:            peer,
			TransactionHash: nil,
			Cheque:          nil,
			Result:          nil,
			UncashedAmount:  bigint.Wrap(uncashedAmount),
		}

		var got *debugapi.SwapCashoutStatusResponse
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/chequebook/cashout/"+addr.String(), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Got: \n %+v \n\n Expected: \n %+v \n\n", got, expected)
		}
	})
}

func LastChequesEqual(a, b *debugapi.ChequebookLastChequesResponse) bool {

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
