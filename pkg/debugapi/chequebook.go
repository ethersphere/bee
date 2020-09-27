// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errChequebookBalance  = "cannot get chequebook balance"
	errCantLastChequePeer = "cannot get last cheque for peer"
	errCantLastCheque     = "cannot get last cheque for all peers"
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

	var lastsentresponse *chequebookLastChequePeerResponse
	lastSent, err := s.Swap.LastSentCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.Logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.Logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastsentresponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastSent.Cheque.Beneficiary.String(),
			Chequebook:  lastSent.Cheque.Chequebook.String(),
			Payout:      lastSent.Cheque.CumulativePayout,
		}
	}

	var lastreceivedresponse *chequebookLastChequePeerResponse
	lastReceived, err := s.Swap.LastReceivedCheque(peer)
	if err != nil && err != chequebook.ErrNoCheque {
		s.Logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.Logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.InternalServerError(w, errCantLastChequePeer)
		return
	}
	if err == nil {
		lastreceivedresponse = &chequebookLastChequePeerResponse{
			Beneficiary: lastReceived.Cheque.Beneficiary.String(),
			Chequebook:  lastReceived.Cheque.Chequebook.String(),
			Payout:      lastReceived.Cheque.CumulativePayout,
		}
	}

	if lastreceivedresponse == nil && lastsentresponse == nil {
		s.Logger.Debugf("debug api: chequebook cheque peer: get peer %s last cheque: %v", peer.String(), err)
		s.Logger.Errorf("debug api: chequebook cheque peer: can't get peer %s last cheque", peer.String())
		jsonhttp.NotFound(w, errCantLastChequePeer)
		return
	}

	jsonhttp.OK(w, chequebookLastChequesPeerResponse{
		Peer:         addr,
		LastReceived: lastreceivedresponse,
		LastSent:     lastsentresponse,
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
