// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	errChequebookBalance           = "cannot get chequebook balance"
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

	gasPriceHeader  = "Gas-Price"
	gasLimitHeader  = "Gas-Limit"
	immutableHeader = "Immutable"
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
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("chequebook balance: get balance failed", "error", err)
		s.logger.Error(nil, "chequebook balance: get balance failed")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.logger.Debug("chequebook balance: get balance failed", "error", err)
		s.logger.Error(nil, "chequebook balance: get balance failed")
		return
	}

	availableBalance, err := s.chequebook.AvailableBalance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.logger.Debug("chequebook balance: get available balance failed", "error", err)
		s.logger.Error(nil, "chequebook balance: get available balance failed")
		return
	}

	jsonhttp.OK(w, chequebookBalanceResponse{TotalBalance: bigint.Wrap(balance), AvailableBalance: bigint.Wrap(availableBalance)})
}

func (s *Service) chequebookAddressHandler(w http.ResponseWriter, r *http.Request) {
	address := s.chequebook.Address()
	jsonhttp.OK(w, chequebookAddressResponse{Address: address.String()})
}

func (s *Service) chequebookLastPeerHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Peer []byte `parse:"peer,addressToBytes" name:"peer" errMessage:"invalid address"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("chequebook cheque peer: invalid peer address string", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "cashout status peer: invalid peer address string", "string", mux.Vars(r)["peer"])
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	peer := swarm.NewAddress(path.Peer)
	var lastSentResponse *chequebookLastChequePeerResponse
	lastSent, err := s.swap.LastSentCheque(peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("chequebook cheque peer: get last sent cheque failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "chequebook cheque peer: get last sent cheque failed", "peer_address", peer)
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil && !errors.Is(err, chequebook.ErrNoCheque) && !errors.Is(err, swap.ErrNoChequebook) {
		s.logger.Debug("chequebook cheque peer: get last sent cheque failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "chequebook cheque peer: get last sent cheque failed", "peer_address", peer)
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
		s.logger.Debug("chequebook cheque peer: get last received cheque failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "chequebook cheque peer: get last received cheque failed", "peer_address", peer)
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
		Peer:         mux.Vars(r)["peer"],
		LastReceived: lastReceivedResponse,
		LastSent:     lastSentResponse,
	})
}

func (s *Service) chequebookAllLastHandler(w http.ResponseWriter, r *http.Request) {
	lastchequessent, err := s.swap.LastSentCheques()
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("chequebook cheque all: get all last sent cheque failed", "error", err)
		s.logger.Error(nil, "chequebook cheque all: get all last sent cheque failed")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		if !errors.Is(err, swap.ErrNoChequebook) {
			s.logger.Debug("chequebook cheque all: get all last sent cheque failed", "error", err)
			s.logger.Error(nil, "chequebook cheque all: get all last sent cheque failed")
			jsonhttp.InternalServerError(w, errCantLastCheque)
			return
		}
		lastchequessent = map[string]*chequebook.SignedCheque{}
	}
	lastchequesreceived, err := s.swap.LastReceivedCheques()
	if err != nil {
		s.logger.Debug("chequebook cheque all: get all last received cheque failed", "error", err)
		s.logger.Error(nil, "chequebook cheque all: get all last received cheque failed")
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
	path := struct {
		Peer []byte `parse:"peer,addressToBytes" name:"postage amount" errMessage:"invalid address"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("cashout peer: invalid peer address string", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "cashout peer: invalid peer address string", "string", mux.Vars(r)["peer"])
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	peer := swarm.NewAddress(path.Peer)
	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "cashout peer: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	if limit, ok := r.Header[gasLimitHeader]; ok {
		l, err := strconv.ParseUint(limit[0], 10, 64)
		if err != nil {
			s.logger.Debug("cashout peer: bad gas limit", "error", err)
			s.logger.Error(nil, "cashout peer: bad gas limit")
			jsonhttp.BadRequest(w, errBadGasLimit)
			return
		}
		ctx = sctx.SetGasLimit(ctx, l)
	}

	if !s.cashOutChequeSem.TryAcquire(1) {
		s.logger.Debug("simultaneous on-chain operations not supported")
		s.logger.Error(nil, "simultaneous on-chain operations not supported")
		jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
		return
	}
	defer s.cashOutChequeSem.Release(1)

	txHash, err := s.swap.CashCheque(ctx, peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("cashout peer: cash cheque failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "cashout peer: cash cheque failed", "peer_address", peer)
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		s.logger.Debug("cashout peer: cash cheque failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "cashout peer: cash cheque failed", "peer_address", peer)
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
	path := struct {
		Peer []byte `parse:"peer,addressToBytes" name:"peer" errMessage:"invalid address"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("cashout status peer: invalid peer address string", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "cashout status peer: invalid peer address string", "string", mux.Vars(r)["peer"])
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	peer := swarm.NewAddress(path.Peer)
	status, err := s.swap.CashoutStatus(r.Context(), peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("cashout status peer: get cashout status failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "cashout status peer: get cashout status failed", "peer_address", peer)
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		if errors.Is(err, chequebook.ErrNoCheque) {
			s.logger.Debug("cashout status peer: get cashout status failed", "peer_address", peer, "error", err)
			s.logger.Error(nil, "cashout status peer: get cashout status failed", "peer_address", peer)
			jsonhttp.NotFound(w, errNoCheque)
			return
		}
		if errors.Is(err, chequebook.ErrNoCashout) {
			s.logger.Debug("cashout status peer: get cashout status failed", "peer_address", peer, "error", err)
			s.logger.Error(nil, "cashout status peer: get cashout status failed", "peer_address", peer)
			jsonhttp.NotFound(w, errNoCashout)
			return
		}
		s.logger.Debug("cashout status peer: get cashout status failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "cashout status peer: get cashout status failed", "peer_address", peer)
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
	path := struct {
		Amount int64 `parse:"amount" name:"amount" errMessage:"did not specify amount"`
	}{}

	if err := s.parseAndValidate(r.URL.Query(), &path); err != nil {
		s.logger.Error(nil, "chequebook withdraw: invalid withdraw amount")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "chequebook withdraw: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	txHash, err := s.chequebook.Withdraw(ctx, big.NewInt(path.Amount))
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		s.logger.Debug("chequebook withdraw: withdraw failed", "error", err)
		s.logger.Error(nil, "chequebook withdraw: withdraw failed")
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookNoWithdraw)
		s.logger.Debug("chequebook withdraw: withdraw failed", "error", err)
		s.logger.Error(nil, "chequebook withdraw: withdraw failed")
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}

func (s *Service) chequebookDepositHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Amount int64 `parse:"amount" name:"amount" errMessage:"did not specify amount"`
	}{}

	if err := s.parseAndValidate(r.URL.Query(), &path); err != nil {
		s.logger.Error(nil, "chequebook deposit: invalid deposit amount")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "chequebook deposit: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	txHash, err := s.chequebook.Deposit(ctx, big.NewInt(path.Amount))
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		s.logger.Debug("chequebook deposit: deposit failed", "error", err)
		s.logger.Error(nil, "chequebook deposit: deposit failed")
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookNoDeposit)
		s.logger.Debug("chequebook deposit: deposit failed", "error", err)
		s.logger.Error(nil, "chequebook deposit: deposit failed")
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}
