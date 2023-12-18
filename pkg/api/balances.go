// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/ethersphere/bee/pkg/log"
	"net/http"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	errCantBalances   = "Cannot get balances"
	errCantBalance    = "Cannot get balance"
	errNoBalance      = "No balance for peer"
	errInvalidAddress = "Invalid address"
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

func (s *Service) balancesHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_consumed").Build()

	balances, err := s.accounting.Balances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		logger.Debug("get balances failed", log.LogItem{"error", err})
		logger.Error(nil, "get balances failed")
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
	logger := s.logger.WithName("get_consumed_by_peer").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	balance, err := s.accounting.Balance(paths.Peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		logger.Debug("get peer balance failed", log.LogItem{"peer_address", paths.Peer}, log.LogItem{"error", err})
		logger.Error(nil, "get peer balance failed", log.LogItem{"peer_address", paths.Peer})
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    paths.Peer.String(),
		Balance: bigint.Wrap(balance),
	})
}

func (s *Service) compensatedBalancesHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_balances").Build()

	balances, err := s.accounting.CompensatedBalances()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantBalances)
		logger.Debug("get compensated balances failed", log.LogItem{"error", err})
		logger.Error(nil, "get compensated balances failed")
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
	logger := s.logger.WithName("get_balances_by_peer").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	balance, err := s.accounting.CompensatedBalance(paths.Peer)
	if err != nil {
		if errors.Is(err, accounting.ErrPeerNoBalance) {
			jsonhttp.NotFound(w, errNoBalance)
			return
		}
		s.logger.Debug("get compensated balances failed", log.LogItem{"peer_address", paths.Peer}, log.LogItem{"error", err})
		s.logger.Error(nil, "get compensated balances failed", log.LogItem{"peer_address", paths.Peer})
		jsonhttp.InternalServerError(w, errCantBalance)
		return
	}

	jsonhttp.OK(w, balanceResponse{
		Peer:    paths.Peer.String(),
		Balance: bigint.Wrap(balance),
	})
}
