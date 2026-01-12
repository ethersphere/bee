// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	errCantSettlements     = "can not get settlements"
	errCantSettlementsPeer = "can not get settlements for peer"
)

type settlementResponse struct {
	Peer               string         `json:"peer"`
	SettlementReceived *bigint.BigInt `json:"received"`
	SettlementSent     *bigint.BigInt `json:"sent"`
}

type settlementsResponse struct {
	TotalSettlementReceived *bigint.BigInt       `json:"totalReceived"`
	TotalSettlementSent     *bigint.BigInt       `json:"totalSent"`
	Settlements             []settlementResponse `json:"settlements"`
}

func (s *Service) settlementsHandler(w http.ResponseWriter, _ *http.Request) {
	resp, err := s.settlements()
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("sent settlements failed", "error", err)
		s.logger.Error(nil, "sent settlements failed")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		s.logger.Debug("sent settlements failed", "error", err)
		s.logger.Error(nil, "sent settlements failed")
		jsonhttp.InternalServerError(w, errCantSettlements)
		return
	}

	jsonhttp.OK(w, resp)
}

func (s *Service) settlements() (*settlementsResponse, error) {
	settlementsSent, err := s.swap.SettlementsSent()
	if err != nil {
		return nil, err
	}
	settlementsReceived, err := s.swap.SettlementsReceived()
	if err != nil {
		return nil, err
	}

	totalReceived := big.NewInt(0)
	totalSent := big.NewInt(0)

	settlementResponses := make(map[string]settlementResponse)

	for a, b := range settlementsSent {
		settlementResponses[a] = settlementResponse{
			Peer:               a,
			SettlementSent:     bigint.Wrap(b),
			SettlementReceived: bigint.Wrap(big.NewInt(0)),
		}
		totalSent.Add(b, totalSent)
	}

	for a, b := range settlementsReceived {
		if _, ok := settlementResponses[a]; ok {
			t := settlementResponses[a]
			t.SettlementReceived = bigint.Wrap(b)
			settlementResponses[a] = t
		} else {
			settlementResponses[a] = settlementResponse{
				Peer:               a,
				SettlementSent:     bigint.Wrap(big.NewInt(0)),
				SettlementReceived: bigint.Wrap(b),
			}
		}
		totalReceived.Add(b, totalReceived)
	}

	settlementResponsesArray := make([]settlementResponse, len(settlementResponses))
	i := 0
	for k := range settlementResponses {
		settlementResponsesArray[i] = settlementResponses[k]
		i++
	}

	return &settlementsResponse{TotalSettlementReceived: bigint.Wrap(totalReceived), TotalSettlementSent: bigint.Wrap(totalSent), Settlements: settlementResponsesArray}, nil
}

func (s *Service) peerSettlementsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_settlement").Build()

	paths := struct {
		Peer swarm.Address `map:"peer" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	resp, err := s.peerSettlements(paths.Peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		logger.Debug("get total received failed", "peer_address", paths.Peer, "error", err)
		logger.Error(nil, "get total received failed", "peer_address", paths.Peer)
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if errors.Is(err, settlement.ErrPeerNoSettlements) {
		jsonhttp.NotFound(w, settlement.ErrPeerNoSettlements)
		return
	}
	if err != nil {
		logger.Debug("get peer settlements failed", "peer_address", paths.Peer, "error", err)
		logger.Error(nil, "get peer settlements failed", "peer_address", paths.Peer)
		jsonhttp.InternalServerError(w, errCantSettlementsPeer)
		return
	}

	jsonhttp.OK(w, resp)
}

func (s *Service) peerSettlements(peer swarm.Address) (*settlementResponse, error) {
	peerexists := false

	received, err := s.swap.TotalReceived(peer)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return nil, err
		} else {
			received = big.NewInt(0)
		}
	}

	if err == nil {
		peerexists = true
	}

	sent, err := s.swap.TotalSent(peer)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return nil, err
		} else {
			sent = big.NewInt(0)
		}
	}

	if err == nil {
		peerexists = true
	}

	if !peerexists {
		return nil, settlement.ErrPeerNoSettlements
	}

	return &settlementResponse{
		Peer:               peer.String(),
		SettlementReceived: bigint.Wrap(received),
		SettlementSent:     bigint.Wrap(sent),
	}, nil
}

func (s *Service) settlementsHandlerPseudosettle(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_timesettlements").Build()

	settlementsSent, err := s.pseudosettle.SettlementsSent()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		logger.Debug("sent settlements failed", "error", err)
		logger.Error(nil, "sent settlements failed")
		return
	}
	settlementsReceived, err := s.pseudosettle.SettlementsReceived()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		logger.Debug("get received settlements failed", "error", err)
		logger.Error(nil, "get received settlements failed")
		return
	}

	totalReceived := big.NewInt(0)
	totalSent := big.NewInt(0)

	settlementResponses := make(map[string]settlementResponse)

	for a, b := range settlementsSent {
		settlementResponses[a] = settlementResponse{
			Peer:               a,
			SettlementSent:     bigint.Wrap(b),
			SettlementReceived: bigint.Wrap(big.NewInt(0)),
		}
		totalSent.Add(b, totalSent)
	}

	for a, b := range settlementsReceived {
		if _, ok := settlementResponses[a]; ok {
			t := settlementResponses[a]
			t.SettlementReceived = bigint.Wrap(b)
			settlementResponses[a] = t
		} else {
			settlementResponses[a] = settlementResponse{
				Peer:               a,
				SettlementSent:     bigint.Wrap(big.NewInt(0)),
				SettlementReceived: bigint.Wrap(b),
			}
		}
		totalReceived.Add(b, totalReceived)
	}

	settlementResponsesArray := make([]settlementResponse, len(settlementResponses))
	i := 0
	for k := range settlementResponses {
		settlementResponsesArray[i] = settlementResponses[k]
		i++
	}

	jsonhttp.OK(w, settlementsResponse{TotalSettlementReceived: bigint.Wrap(totalReceived), TotalSettlementSent: bigint.Wrap(totalSent), Settlements: settlementResponsesArray})
}
