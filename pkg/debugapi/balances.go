// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errCantBalances  = "Cannot get balances"
	errCantBalance   = "Cannot get balance"
	errNoBalance     = "No balance for peer"
	errInvaliAddress = "Invalid address"
)

type balanceResponse struct {
	Peer    string `json:"peer"`
	Balance int64  `json:"balance"`
}

type balancesResponse struct {
	Balances []balanceResponse `json:"balances"`
}

func (s *server) balancesHandler(w http.ResponseWriter, r *http.Request) {
	balances, err := s.Accounting.Balances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		s.Logger.Debugf("debug api: balances: %v", err)
		s.Logger.Error("debug api: can not get balances")
		return
	}

	balResponses := make([]balanceResponse, len(balances))
	i := 0
	for k := range balances {
		balResponses[i] = balanceResponse{
			Peer:    k,
			Balance: balances[k],
		}
		i++
	}

	jsonhttp.OK(w, balancesResponse{Balances: balResponses})
}

func (s *server) peerBalanceHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: balances peer: invalid peer address %s: %v", addr, err)
		s.Logger.Errorf("debug api: balances peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	balance, err := s.Accounting.Balance(peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		s.Logger.Debugf("debug api: balances peer: get peer %s balance: %v", peer.String(), err)
		s.Logger.Errorf("debug api: balances peer: can't get peer %s balance", peer.String())
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: balance,
	})
}

func (s *server) compensatedBalancesHandler(w http.ResponseWriter, r *http.Request) {
	balances, err := s.Accounting.CompensatedBalances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		s.Logger.Debugf("debug api: compensated balances: %v", err)
		s.Logger.Error("debug api: can not get compensated balances")
		return
	}

	balResponses := make([]balanceResponse, len(balances))
	i := 0
	for k := range balances {
		balResponses[i] = balanceResponse{
			Peer:    k,
			Balance: balances[k],
		}
		i++
	}

	jsonhttp.OK(w, balancesResponse{Balances: balResponses})
}

func (s *server) compensatedPeerBalanceHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: compensated balances peer: invalid peer address %s: %v", addr, err)
		s.Logger.Errorf("debug api: compensated balances peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	balance, err := s.Accounting.CompensatedBalance(peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		s.Logger.Debugf("debug api: compensated balances peer: get peer %s balance: %v", peer.String(), err)
		s.Logger.Errorf("debug api: compensated balances peer: can't get peer %s balance", peer.String())
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: balance,
	})
}
