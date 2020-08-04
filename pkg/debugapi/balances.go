// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"

	"net/http"
)

var (
	errCantBalances  = "Can not get balances"
	errCantBalance   = "Can not get balance"
	errMalformedPeer = "Malformed peer address"
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
		s.Logger.Error("debug api: Can not get balances")
		return
	}

	var balResponses []balanceResponse

	for k := range balances {
		balResponses = append(balResponses, balanceResponse{
			Peer:    k,
			Balance: balances[k],
		})
	}

	jsonhttp.OK(w, balancesResponse{Balances: balResponses})

}

func (s *server) peerBalanceHandler(w http.ResponseWriter, r *http.Request) {
	peer, err := swarm.ParseHexAddress(mux.Vars(r)["peer"])
	if err != nil {
		s.Logger.Debugf("debug api: balances peer: parse peer address: %v", err)
		s.Logger.Error("debug api: balances peer: Can't parse peer address")
		jsonhttp.BadRequest(w, errMalformedPeer)
		return
	}

	balance, err := s.Accounting.Balance(peer)

	if err != nil {
		s.Logger.Debugf("debug api: balances peer: get peer balance: %v", err)
		s.Logger.Error("debug api: balances peer: Can't get peer balance")
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: balance,
	})

}
