// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
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
	logger := s.logger.WithName("get_chequebook_balance").Build()

	balance, err := s.chequebook.Balance(r.Context())
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("unable to get balance", "error", err)
		logger.Warning("unable to get balance")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		logger.Debug("unable to get balance", "error", err)
		logger.Warning("unable to get balance")
		return
	}

	availableBalance, err := s.chequebook.AvailableBalance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		logger.Debug("unable to get available balance", "error", err)
		logger.Warning("unable to get available balance")
		return
	}

	jsonhttp.OK(w, chequebookBalanceResponse{TotalBalance: bigint.Wrap(balance), AvailableBalance: bigint.Wrap(availableBalance)})
}

func (s *Service) chequebookAddressHandler(w http.ResponseWriter, _ *http.Request) {
	jsonhttp.OK(w, chequebookAddressResponse{Address: s.chequebook.Address().String()})
}

func (s *Service) chequebookLastPeerHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_chequebook_cheque_by_peer").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	var lastSentResponse *chequebookLastChequePeerResponse
	lastSent, err := s.swap.LastSentCheque(paths.Peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("unable to get last sent cheque", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to get last sent cheque")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil && !errors.Is(err, chequebook.ErrNoCheque) && !errors.Is(err, swap.ErrNoChequebook) {
		logger.Debug("unable to get last sent cheque", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to get last sent cheque")
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
	lastReceived, err := s.swap.LastReceivedCheque(paths.Peer)
	if err != nil && !errors.Is(err, chequebook.ErrNoCheque) {
		logger.Debug("unable to get last received cheque", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to get last received cheque")
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
		Peer:         paths.Peer.String(),
		LastReceived: lastReceivedResponse,
		LastSent:     lastSentResponse,
	})
}

func (s *Service) chequebookAllLastHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_chequebook_cheques").Build()

	lastchequessent, err := s.swap.LastSentCheques()
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("unable to get all last sent cheques", "error", err)
		logger.Warning("unable to get all last sent cheques")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		if !errors.Is(err, swap.ErrNoChequebook) {
			logger.Debug("unable to get all last sent cheques", "error", err)
			logger.Warning("unable to get all last sent cheques")
			jsonhttp.InternalServerError(w, errCantLastCheque)
			return
		}
		lastchequessent = map[string]*chequebook.SignedCheque{}
	}
	lastchequesreceived, err := s.swap.LastReceivedCheques()
	if err != nil {
		logger.Debug("unable to get all last received cheques", "error", err)
		logger.Debug("unable to get all last received cheques")
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
	logger := s.logger.WithName("post_chequebook_cashout").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	if !s.cashOutChequeSem.TryAcquire(1) {
		logger.Debug("simultaneous on-chain operations not supported")
		logger.Warning("simultaneous on-chain operations not supported")
		jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
		return
	}
	defer s.cashOutChequeSem.Release(1)

	txHash, err := s.swap.CashCheque(r.Context(), paths.Peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("unable to cashout cheque", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to cashout cheque")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		logger.Debug("unable to cashout cheque", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to cashout cheque")
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
	logger := s.logger.WithName("get_chequebook_cashout").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	status, err := s.swap.CashoutStatus(r.Context(), paths.Peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("unable to get cashout status", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to get cashout status")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		if errors.Is(err, chequebook.ErrNoCheque) {
			logger.Debug("unable to get cashout status", "peer_address", paths.Peer, "error", err)
			logger.Warning("unable to get cashout status")
			jsonhttp.NotFound(w, errNoCheque)
			return
		}
		if errors.Is(err, chequebook.ErrNoCashout) {
			logger.Debug("unable to get cashout status", "peer_address", paths.Peer, "error", err)
			logger.Warning("unable to get cashout status")
			jsonhttp.NotFound(w, errNoCashout)
			return
		}
		logger.Debug("unable to get cashout status", "peer_address", paths.Peer, "error", err)
		logger.Warning("unable to get cashout status")
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
		Peer:            paths.Peer,
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
	logger := s.logger.WithName("post_chequebook_withdraw").Build()

	queries := struct {
		Amount *big.Int `map:"amount" validate:"required"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	txHash, err := s.chequebook.Withdraw(r.Context(), queries.Amount)
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		logger.Debug("unable to withdraw from chequebook", "error", err)
		logger.Warning("unable to withdraw from chequebook")
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		return
	}
	if err != nil {
		logger.Debug("unable to withdraw from chequebook", "error", err)
		logger.Warning("unable to withdraw from chequebook")
		jsonhttp.InternalServerError(w, errChequebookNoWithdraw)
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}

func (s *Service) chequebookDepositHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_chequebook_deposit").Build()

	queries := struct {
		Amount *big.Int `map:"amount" validate:"required"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	txHash, err := s.chequebook.Deposit(r.Context(), queries.Amount)
	if errors.Is(err, chequebook.ErrInsufficientFunds) {
		logger.Debug("unable to make chequebook deposit", "error", err)
		logger.Warning("unable to make chequebook deposit")
		jsonhttp.BadRequest(w, errChequebookInsufficientFunds)
		return
	}
	if err != nil {
		logger.Debug("unable to make chequebook deposit", "error", err)
		logger.Warning("unable to make chequebook deposit")
		jsonhttp.InternalServerError(w, errChequebookNoDeposit)
		return
	}

	jsonhttp.OK(w, chequebookTxResponse{TransactionHash: txHash})
}
