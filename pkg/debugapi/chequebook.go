// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errChequebookBalance  = "cannot get chequebook balance"
	errCantLastChequePeer = "cannot get last cheque for peer"
	errCantLastCheque     = "cannot get last cheque for all peers"
	errCannotCash         = "cannot cash cheque"
	errCannotCashStatus   = "cannot get cashout status"
)

type chequebookBalanceResponse struct {
	TotalBalance     *big.Int `json:"totalBalance"`
	AvailableBalance *big.Int `json:"availableBalance"`
}

type chequebookAddressResponse struct {
	Address string `json:"chequebookaddress"`
}

type chequebookLastChequePeerResponse struct {
	Beneficiary string   `json:"beneficiary"`
	Chequebook  string   `json:"chequebook"`
	Payout      *big.Int `json:"payout"`
}

type chequebookLastChequesPeerResponse struct {
	Peer         string                            `json:"peer"`
	LastReceived *chequebookLastChequePeerResponse `json:"lastreceived"`
	LastSent     *chequebookLastChequePeerResponse `json:"lastsent"`
}

type chequebookLastChequesResponse struct {
	LastCheques []chequebookLastChequesPeerResponse `json:"lastcheques"`
}

func (s *server) chequebookBalanceHandler(w http.ResponseWriter, r *http.Request) {
	balance, err := s.Chequebook.Balance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.Logger.Debugf("debug api: chequebook balance: %v", err)
		s.Logger.Error("debug api: cannot get chequebook balance")
		return
	}

	availableBalance, err := s.Chequebook.AvailableBalance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.Logger.Debugf("debug api: chequebook availableBalance: %v", err)
		s.Logger.Error("debug api: cannot get chequebook availableBalance")
		return
	}

	jsonhttp.OK(w, chequebookBalanceResponse{TotalBalance: balance, AvailableBalance: availableBalance})
}

func (s *server) chequebookAddressHandler(w http.ResponseWriter, r *http.Request) {
	address := s.Chequebook.Address()
	jsonhttp.OK(w, chequebookAddressResponse{Address: address.String()})
}

func (s *server) chequebookLastPeerHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: chequebook cheque peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: chequebook cheque peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	var lastSentResponse *chequebookLastChequePeerResponse
	lastSent, err := s.Swap.LastSentCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.Logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.Logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastSentResponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastSent.Cheque.Beneficiary.String(),
			Chequebook:  lastSent.Cheque.Chequebook.String(),
			Payout:      lastSent.Cheque.CumulativePayout,
		}
	}

	var lastReceivedResponse *chequebookLastChequePeerResponse
	lastReceived, err := s.Swap.LastReceivedCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.Logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.Logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastReceivedResponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastReceived.Cheque.Beneficiary.String(),
			Chequebook:  lastReceived.Cheque.Chequebook.String(),
			Payout:      lastReceived.Cheque.CumulativePayout,
		}
	}

	jsonhttp.OK(w, chequebookLastChequesPeerResponse{
		Peer:         addr,
		LastReceived: lastReceivedResponse,
		LastSent:     lastSentResponse,
	})
}

func (s *server) chequebookAllLastHandler(w http.ResponseWriter, r *http.Request) {
	lastchequessent, err := s.Swap.LastSentCheques()
	if err != nil {
		s.Logger.Debugf("debug api: chequebook cheque all: get all last cheques: %v", err)
		s.Logger.Errorf("debug api: chequebook cheque all: can't get all last cheques")
		jsonhttp.InternalServerError(w, errCantLastCheque)
		return
	}
	lastchequesreceived, err := s.Swap.LastReceivedCheques()
	if err != nil {
		s.Logger.Debugf("debug api: chequebook cheque all: get all last cheques: %v", err)
		s.Logger.Errorf("debug api: chequebook cheque all: can't get all last cheques")
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
				Payout:      j.Cheque.CumulativePayout,
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
				Payout:      j.Cheque.CumulativePayout,
			}
			lcr[i] = t
		} else {
			lcr[i] = chequebookLastChequesPeerResponse{
				Peer:     i,
				LastSent: nil,
				LastReceived: &chequebookLastChequePeerResponse{
					Beneficiary: j.Cheque.Beneficiary.String(),
					Chequebook:  j.Cheque.Chequebook.String(),
					Payout:      j.Cheque.CumulativePayout,
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

func (s *server) swapCashoutHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: cashout peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: cashout peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	txHash, err := s.Swap.CashCheque(r.Context(), peer)
	if err != nil {
		s.Logger.Debugf("debug api: cashout peer: cannot cash %s: %v", addr, err)
		s.Logger.Error("debug api: cashout peer: cannot cash %s", addr)
		jsonhttp.NotFound(w, errCannotCash)
		return
	}

	jsonhttp.OK(w, swapCashoutResponse{TransactionHash: txHash.String()})
}

type swapCashoutStatusResult struct {
	Recipient  common.Address `json:"recipient"`
	LastPayout *big.Int       `json:"lastPayout"`
	Bounced    bool           `json:"bounced"`
}

type swapCashoutStatusResponse struct {
	Peer             swarm.Address            `json:"peer"`
	Chequebook       common.Address           `json:"chequebook"`
	CumulativePayout *big.Int                 `json:"cumulativePayout"`
	Beneficiary      common.Address           `json:"beneficiary"`
	TransactionHash  common.Hash              `json:"transactionHash"`
	Result           *swapCashoutStatusResult `json:"result"`
}

func (s *server) swapCashoutStatusHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: cashout status peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: cashout status peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	status, err := s.Swap.CashoutStatus(r.Context(), peer)
	if err != nil {
		s.Logger.Debugf("debug api: cashout status peer: cannot get status %s: %v", addr, err)
		s.Logger.Error("debug api: cashout status peer: cannot get status %s", addr)
		jsonhttp.NotFound(w, errCannotCashStatus)
		return
	}

	var result *swapCashoutStatusResult
	if status.Result != nil {
		result = &swapCashoutStatusResult{
			Recipient:  status.Result.Recipient,
			LastPayout: status.Result.TotalPayout,
			Bounced:    status.Result.Bounced,
		}
	}

	jsonhttp.OK(w, swapCashoutStatusResponse{
		Peer:             peer,
		TransactionHash:  status.TxHash,
		Chequebook:       status.Cheque.Chequebook,
		CumulativePayout: status.Cheque.CumulativePayout,
		Beneficiary:      status.Cheque.Beneficiary,
		Result:           result,
	})
}
