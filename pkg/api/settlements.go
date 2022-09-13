// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/mux"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/swarm"
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

func (s *Service) settlementsHandler(w http.ResponseWriter, r *http.Request) {

	settlementsSent, err := s.swap.SettlementsSent()
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("settlements get: sent settlements failed", "error", err)
		s.logger.Error(nil, "settlements get: sent settlements failed")
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		s.logger.Debug("settlements get: sent settlements failed", "error", err)
		s.logger.Error(nil, "settlements get: sent settlements failed")
		jsonhttp.InternalServerError(w, errCantSettlements)
		return
	}
	settlementsReceived, err := s.swap.SettlementsReceived()
	if err != nil {
		s.logger.Debug("settlements get: get received settlements failed", "error", err)
		s.logger.Error(nil, "settlements get: get received settlements failed")
		jsonhttp.InternalServerError(w, errCantSettlements)
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

func (s *Service) peerSettlementsHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Peer []byte `parse:"peer,addressToString" name:"peer"`
	}{}
	err := s.parseAndValidate(r, &path)
	if err != nil {
		s.logger.Debug("settlements peer: parse peer address string failed", "string", mux.Vars(r)["peer"], "error", err)
		s.logger.Error(nil, "settlements peer: parse peer address string failed", "string", mux.Vars(r)["peer"])
		jsonhttp.NotFound(w, errInvalidAddress)
		return
	}
	peerexists := false

	peer := swarm.NewAddress(path.Peer)
	received, err := s.swap.TotalReceived(peer)
	if errors.Is(err, postagecontract.ErrChainDisabled) {
		s.logger.Debug("settlements peer: get total received failed", "peer_address", peer, "error", err)
		s.logger.Error(nil, "settlements peer: get total received failed", "peer_address", peer)
		jsonhttp.MethodNotAllowed(w, err)
		return
	}
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			s.logger.Debug("settlements peer: get total received failed", "peer_address", peer, "error", err)
			s.logger.Error(nil, "settlements peer: get total received failed", "peer_address", peer)
			jsonhttp.InternalServerError(w, errCantSettlementsPeer)
			return
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
			s.logger.Debug("settlements peer: get total sent failed", "peer_address", peer, "error", err)
			s.logger.Error(nil, "settlements peer: get total sent failed", "peer_address", peer)
			jsonhttp.InternalServerError(w, errCantSettlementsPeer)
			return
		} else {
			sent = big.NewInt(0)
		}
	}

	if err == nil {
		peerexists = true
	}

	if !peerexists {
		jsonhttp.NotFound(w, settlement.ErrPeerNoSettlements)
		return
	}

	jsonhttp.OK(w, settlementResponse{
		Peer:               peer.String(),
		SettlementReceived: bigint.Wrap(received),
		SettlementSent:     bigint.Wrap(sent),
	})
}

func (s *Service) settlementsHandlerPseudosettle(w http.ResponseWriter, r *http.Request) {

	settlementsSent, err := s.pseudosettle.SettlementsSent()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		s.logger.Debug("time settlements: sent settlements failed", "error", err)
		s.logger.Error(nil, "time settlements: sent settlements failed")
		return
	}
	settlementsReceived, err := s.pseudosettle.SettlementsReceived()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		s.logger.Debug("time settlements: get received settlements failed", "error", err)
		s.logger.Error(nil, "time settlements: get received settlements failed")
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
