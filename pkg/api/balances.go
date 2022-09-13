// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/mux"
	"net/http"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	errCantBalances   = "Cannot get balances"
	errCantBalance    = "Cannot get balance"
	errNoBalance      = "No balance for peer"
	errInvalidAddress = "invalid address"
)

type balanceResponse struct {
	Peer              string         `json:"peer"`
	Balance           *bigint.BigInt `json:"balance"`
	ThresholdReceived *bigint.BigInt `json:"thresholdreceived"`
	ThresholdGiven    *bigint.BigInt `json:"thresholdgiven"`
}

type balancesResponse struct {
	Balances []balanceResponse `json:"balances"`
}

func (s *Service) balancesHandler(w http.ResponseWriter, r *http.Request) {
	balances, err := s.accounting.Balances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		s.logger.Debug("balances: get balances failed", "error", err)
		s.logger.Error(nil, "balances: get balances failed")
		return
	}

	balResponses := make([]balanceResponse, len(balances))
	i := 0
	for k := range balances {
		balResponses[i] = balanceResponse{
			Peer:    k,
			Balance: bigint.Wrap(balances[k]),
		}
		i++
	}

	jsonhttp.OK(w, balancesResponse{Balances: balResponses})
}

func (s *Service) peerBalanceHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Peer []byte `parse:"peer,addressToString" name:"address"`
	}{}

	if err := s.parseAndValidate(r, &path); err != nil {
		s.logger.Debug("balances peer: parse address string failed", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "balances peer: parse address string failed", "string", mux.Vars(r)["peer"])
		jsonhttp.NotFound(w, err.Error())
		return
	}
	peer := swarm.NewAddress(path.Peer)

	balance, err := s.accounting.Balance(peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		s.logger.Debug("balances peer: get peer balance failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "balances peer: get peer balance failed", "peer_address", peer)
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: bigint.Wrap(balance),
	})
}

func (s *Service) compensatedBalancesHandler(w http.ResponseWriter, r *http.Request) {
	balances, err := s.accounting.CompensatedBalances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		s.logger.Debug("compensated balances: get compensated balances failed", "error", err)
		s.logger.Error(nil, "compensated balances: get compensated balances failed")
		return
	}

	balResponses := make([]balanceResponse, len(balances))
	i := 0
	for k := range balances {
		balResponses[i] = balanceResponse{
			Peer:    k,
			Balance: bigint.Wrap(balances[k]),
		}
		i++
	}

	jsonhttp.OK(w, balancesResponse{Balances: balResponses})
}

func (s *Service) compensatedPeerBalanceHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Peer []byte `parse:"peer,addressToString" name:"address"`
	}{}

	if err := s.parseAndValidate(r, &path); err != nil {
		s.logger.Debug("compensated balances peer: parse address string failed", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "compensated balances peer: parse address string failed", "string", mux.Vars(r)["peer"])
		jsonhttp.NotFound(w, err.Error())
		return
	}
	peer := swarm.NewAddress(path.Peer)

	balance, err := s.accounting.CompensatedBalance(peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		s.logger.Debug("compensated balances peer: get compensated balances failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "compensated balances peer: get compensated balances failed", "peer_address", peer)
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    peer.String(),
		Balance: bigint.Wrap(balance),
	})
}
