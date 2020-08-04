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
		jsonhttp.InternalServerError(w, "Can not get balances")
		s.Logger.Debugf("debug api: balances: %v", err)
		s.Logger.Errorf("debug api: balances: %v", err)
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
	// TODO: Currently, we do not check the length of the hex address (such as prepending zeroes), should we?
	peer, err := swarm.ParseHexAddress(mux.Vars(r)["peer"])
	if err != nil {
		s.Logger.Debugf("debug api: balances peer: parse peer address: %v", err)
		jsonhttp.BadRequest(w, "malformed peer address")
		return
	}

	balance, err := s.Accounting.Balance(peer)

	if err != nil {
		s.Logger.Debugf("debug-api: balances peer: get peer balance: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: balance,
	})

}
