// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errChequebookBalance           = "cannot get chequebook balance"
	errChequebookNoAmount          = "did not specify amount"
	errChequebookNoWithdraw        = "cannot withdraw"
	errChequebookNoDeposit         = "cannot deposit"
	errChequebookInsufficientFunds = "insufficient funds"
	errCantLastChequePeer          = "cannot get last cheque for peer"
	errCantLastCheque              = "cannot get last cheque for all peers"
	errCannotCash                  = "cannot cash cheque"
	errCannotCashStatus            = "cannot get cashout status"
	errNoCashout                   = "no prior cashout"
	errNoCheque                    = "no prior cheque"
	errBadGasPrice                 = "bad gas price"
	errBadGasLimit                 = "bad gas limit"

	gasPriceHeader = "Gas-Price"
	gasLimitHeader = "Gas-Limit"
)

type chequebookBalanceResponse struct {
	TotalBalance     *bigint.BigInt `json:"totalBalance"`
	AvailableBalance *bigint.BigInt `json:"availableBalance"`
}

type chequebookAddressResponse struct {
	Address string `json:"chequebookAddress"`
}

type chequebookLastChequePeerResponse struct {
	Beneficiary string         `json:"beneficiary"`
	Chequebook  string         `json:"chequebook"`
	Payout      *bigint.BigInt `json:"payout"`
}

type chequebookLastChequesPeerResponse struct {
	Peer         string                            `json:"peer"`
	LastReceived *chequebookLastChequePeerResponse `json:"lastreceived"`
	LastSent     *chequebookLastChequePeerResponse `json:"lastsent"`
}

type chequebookLastChequesResponse struct {
	LastCheques []chequebookLastChequesPeerResponse `json:"lastcheques"`
}

func (s *Service) chequebookBalanceHandler(w http.ResponseWriter, r *http.Request) {
	balance, err := s.chequebook.Balance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.logger.Debugf("debug api: chequebook balance: %v", err)
		s.logger.Error("debug api: cannot get chequebook balance")
		return
	}

	availableBalance, err := s.chequebook.AvailableBalance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.logger.Debugf("debug api: chequebook availableBalance: %v", err)
		s.logger.Error("debug api: cannot get chequebook availableBalance")
		return
	}

	jsonhttp.OK(w, chequebookBalanceResponse{TotalBalance: bigint.Wrap(balance), AvailableBalance: bigint.Wrap(availableBalance)})
}

func (s *Service) chequebookAddressHandler(w http.ResponseWriter, r *http.Request) {
	address := s.chequebook.Address()
	jsonhttp.OK(w, chequebookAddressResponse{Address: address.String()})
}

func (s *Service) chequebookLastPeerHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.logger.Debugf("debug api: chequebook cheque peer: invalid peer address %s: %v", addr, err)
		s.logger.Errorf("debug api: chequebook cheque peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvalidAddress)
		return
	}

	var lastSentResponse *chequebookLastChequePeerResponse
	lastSent, err := s.swap.LastSentCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastSentResponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastSent.Cheque.Beneficiary.String(),
			Chequebook:  lastSent.Cheque.Chequebook.String(),
			Payout:      bigint.Wrap(lastSent.Cheque.CumulativePayout),
		}
	}

	var lastReceivedResponse *chequebookLastChequePeerResponse
	lastReceived, err := s.swap.LastReceivedCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastReceivedResponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastReceived.Cheque.Beneficiary.String(),
			Chequebook:  lastReceived.Cheque.Chequebook.String(),
			Payout:      bigint.Wrap(lastReceived.Cheque.CumulativePayout),
		}
	}

	jsonhttp.OK(w, chequebookLastChequesPeerResponse{
		Peer:         addr,
		LastReceived: lastReceivedResponse,
		LastSent:     lastSentResponse,
	})
}

func (s *Service) chequebookAllLastHandler(w http.ResponseWriter, r *http.Request) {
	lastchequessent, err := s.swap.LastSentCheques()
	if err != nil {
		s.logger.Debugf("debug api: chequebook cheque all: get all last cheques: %v", err)
		s.logger.Errorf("debug api: chequebook cheque all: can't get all last cheques")
		jsonhttp.InternalServerError(w, errCantLastCheque)
		return
	}
	lastchequesreceived, err := s.swap.LastReceivedCheques()
	if err != nil {
		s.logger.Debugf("debug api: chequebook cheque all: get all last cheques: %v", err)
		s.logger.Errorf("debug api: chequebook cheque all: can't get all last cheques")
		jsonhttp.InternalServerError(w, errCantLastCheque)
		return
	}

	lcr := make(map[string]chequebookLastChequesPeerResponse)
	for i, j := range lastchequessent {
		lcr[i] = chequebookLastChequesPeerResponse{
			Peer: i,
			LastSent: &chequebookLastChequePeerResponse{
				Beneficiary: j.Cheque.Beneficiary.String(),
				Chequebook:  j.Cheque.Chequebook.String(),
				Payout:      bigint.Wrap(j.Cheque.CumulativePayout),
			},
			LastReceived: nil,
		}
	}
	for i, j := range lastchequesreceived {
		if _, ok := lcr[i]; ok {
			t := lcr[i]
			t.LastReceived = &chequebookLastChequePeerResponse{
				Beneficiary: j.Cheque.Beneficiary.String(),
				Chequebook:  j.Cheque.Chequebook.String(),
				Payout:      bigint.Wrap(j.Cheque.CumulativePayout),
			}
			lcr[i] = t
		} else {
			lcr[i] = chequebookLastChequesPeerResponse{
				Peer:     i,
				LastSent: nil,
				LastReceived: &chequebookLastChequePeerResponse{
					Beneficiary: j.Cheque.Beneficiary.String(),
					Chequebook:  j.Cheque.Chequebook.String(),
					Payout:      bigint.Wrap(j.Cheque.CumulativePayout),
				},
			}
		}
	}

	lcresponses := make([]chequebookLastChequesPeerResponse, len(lcr))
	i := 0
	for k := range lcr {
		lcresponses[i] = lcr[k]
		i++
	}

	jsonhttp.OK(w, chequebookLastChequesResponse{LastCheques: lcresponses})
}

type swapCashoutResponse struct {
	TransactionHash string `json:"transactionHash"`
}

func (s *Service) swapCashoutHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.logger.Debugf("debug api: cashout peer: invalid peer address %s: %v", addr, err)
		s.logger.Errorf("debug api: cashout peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvalidAddress)
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("debug api: cashout peer: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	if limit, ok := r.Header[gasLimitHeader]; ok {
		l, err := strconv.ParseUint(limit[0], 10, 64)
		if err != nil {
			s.logger.Debugf("debug api: cashout peer: bad gas limit: %v", err)
			s.logger.Error("debug api: cashout peer: bad gas limit")
			jsonhttp.BadRequest(w, errBadGasLimit)
			return
		}
		ctx = sctx.SetGasLimit(ctx, l)
	}

	txHash, err := s.swap.CashCheque(ctx, peer)
	if err != nil {
		s.logger.Debugf("debug api: cashout peer: cannot cash %s: %v", addr, err)
		s.logger.Errorf("debug api: cashout peer: cannot cash %s", addr)
		jsonhttp.InternalServerError(w, errCannotCash)
		return
	}

	jsonhttp.OK(w, swapCashoutResponse{TransactionHash: txHash.String()})
}

type swapCashoutStatusResult struct {
	Recipient  common.Address `json:"recipient"`
	LastPayout *bigint.BigInt `json:"lastPayout"`
	Bounced    bool           `json:"bounced"`
}

type swapCashoutStatusResponse struct {
	Peer            swarm.Address                     `json:"peer"`
	Cheque          *chequebookLastChequePeerResponse `json:"lastCashedCheque"`
	TransactionHash *common.Hash                      `json:"transactionHash"`
	Result          *swapCashoutStatusResult          `json:"result"`
	UncashedAmount  *bigint.BigInt                    `json:"uncashedAmount"`
}

func (s *Service) swapCashoutStatusHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.logger.Debugf("debug api: cashout status peer: invalid peer address %s: %v", addr, err)
		s.logger.Errorf("debug api: cashout status peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvalidAddress)
		return
	}

	status, err := s.swap.CashoutStatus(r.Context(), peer)
	if err != nil {
		if errors.Is(err, chequebook.ErrNoCheque) {
			s.logger.Debugf("debug api: cashout status peer: %v", addr, err)
			s.logger.Errorf("debug api: cashout status peer: %s", addr)
			jsonhttp.NotFound(w, errNoCheque)
			return
		}
		if errors.Is(err, chequebook.ErrNoCashout) {
			s.logger.Debugf("debug api: cashout status peer: %v", addr, err)
			s.logger.Errorf("debug api: cashout status peer: %s", addr)
			jsonhttp.NotFound(w, errNoCashout)
			return
		}
		s.logger.Debugf("debug api: cashout status peer: cannot get status %s: %v", addr, err)
		s.logger.Errorf("debug api: cashout status peer: cannot get status %s", addr)
		jsonhttp.InternalServerError(w, errCannotCashStatus)
		return
	}

	var result *swapCashoutStatusResult
	var txHash *common.Hash
	var chequeResponse *chequebookLastChequePeerResponse
	if status.Last != nil {
		if status.Last.Result != nil {
			result = &swapCashoutStatusResult{
				Recipient:  status.Last.Result.Recipient,
				LastPayout: bigint.Wrap(status.Last.Result.TotalPayout),
				Bounced:    status.Last.Result.Bounced,
			}
		}
		chequeResponse = &chequebookLastChequePeerResponse{
			Chequebook:  status.Last.Cheque.Chequebook.String(),
			Payout:      bigint.Wrap(status.Last.Cheque.CumulativePayout),
			Beneficiary: status.Last.Cheque.Beneficiary.String(),
		}
		txHash = &status.Last.TxHash
	}

	jsonhttp.OK(w, swapCashoutStatusResponse{
		Peer:            peer,
		TransactionHash: txHash,
		Cheque:          chequeResponse,
		Result:          result,
		UncashedAmount:  bigint.Wrap(status.UncashedAmount),
	})
}

type chequebookTxResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

func (s *Service) chequebookWithdrawHandler(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	if amountStr == "" {
		jsonhttp.BadRequest(w, errChequebookNoAmount)
		s.logger.Error("debug api: no withdraw amount")
		return
	}

	amount, ok := big.NewInt(0).SetString(amountStr, 10)
	if !ok {
		jsonhttp.BadRequest(w, errChequebookNoAmount)
		s.logger.Error("debug api: invalid withdraw amount")
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("debug api: withdraw: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	txHash, err := s.chequebook.Withdraw(ctx, amount)
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		s.logger.Debugf("debug api: chequebook withdraw: %v", err)
		s.logger.Error("debug api: cannot withdraw from chequebook")
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookNoWithdraw)
		s.logger.Debugf("debug api: chequebook withdraw: %v", err)
		s.logger.Error("debug api: cannot withdraw from chequebook")
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}

func (s *Service) chequebookDepositHandler(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	if amountStr == "" {
		jsonhttp.BadRequest(w, errChequebookNoAmount)
		s.logger.Error("debug api: no deposit amount")
		return
	}

	amount, ok := big.NewInt(0).SetString(amountStr, 10)
	if !ok {
		jsonhttp.BadRequest(w, errChequebookNoAmount)
		s.logger.Error("debug api: invalid deposit amount")
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("debug api: deposit: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	txHash, err := s.chequebook.Deposit(ctx, amount)
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		s.logger.Debugf("debug api: chequebook deposit: %v", err)
		s.logger.Error("debug api: cannot deposit from chequebook")
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookNoDeposit)
		s.logger.Debugf("debug api: chequebook deposit: %v", err)
		s.logger.Error("debug api: cannot deposit from chequebook")
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}
