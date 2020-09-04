// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errCantSettlements     = "Cannot get settlements"
	errCantSettlementsPeer = "Cannot get settlements for peer"
)

type settlementResponse struct {
	Peer               string `json:"peer"`
	SettlementReceived uint64 `json:"received"`
	SettlementSent     uint64 `json:"sent"`
}

type settlementsResponse struct {
	TotalSettlementReceived *big.Int             `json:"totalreceived"`
	TotalSettlementSent     *big.Int             `json:"totalsent"`
	Settlements             []settlementResponse `json:"settlements"`
}

func (s *server) settlementsHandler(w http.ResponseWriter, r *http.Request) {

	settlementsSent, err := s.Settlement.SettlementsSent()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		s.Logger.Debugf("debug api: sent settlements: %v", err)
		s.Logger.Error("debug api: can not get sent settlements")
		return
	}
	settlementsReceived, err := s.Settlement.SettlementsReceived()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantSettlements)
		s.Logger.Debugf("debug api: received settlements: %v", err)
		s.Logger.Error("debug api: can not get received settlements")
		return
	}

	totalReceived := big.NewInt(0)
	totalSent := big.NewInt(0)

	settlementResponses := make(map[string]settlementResponse)

	for a, b := range settlementsSent {
		settlementResponses[a] = settlementResponse{
			Peer:               a,
			SettlementSent:     b,
			SettlementReceived: 0,
		}
		totalSent.Add(big.NewInt(int64(b)), totalSent)
	}

	for a, b := range settlementsReceived {
		if _, ok := settlementResponses[a]; ok {
			t := settlementResponses[a]
			t.SettlementReceived = b
			settlementResponses[a] = t
		} else {
			settlementResponses[a] = settlementResponse{
				Peer:               a,
				SettlementSent:     0,
				SettlementReceived: b,
			}
		}
		totalReceived.Add(big.NewInt(int64(b)), totalReceived)
	}

	settlementResponsesArray := make([]settlementResponse, len(settlementResponses))
	i := 0
	for k := range settlementResponses {
		settlementResponsesArray[i] = settlementResponses[k]
		i++
	}

	jsonhttp.OK(w, settlementsResponse{TotalSettlementReceived: totalReceived, TotalSettlementSent: totalSent, Settlements: settlementResponsesArray})
}

func (s *server) peerSettlementsHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["peer"]
	peer, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("debug api: settlements peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: settlements peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	peerexists := false

	received, err := s.Settlement.TotalReceived(peer)
	if err != nil {
		if !errors.Is(err, pseudosettle.ErrPeerNoSettlements) {
			s.Logger.Debugf("debug api: settlements peer: get peer %s received settlement: %v", peer.String(), err)
			s.Logger.Errorf("debug api: settlements peer: can't get peer %s received settlement", peer.String())
			jsonhttp.InternalServerError(w, errCantSettlementsPeer)
			return
		}
	}

	if err == nil {
		peerexists = true
	}

	sent, err := s.Settlement.TotalSent(peer)
	if err != nil {
		if !errors.Is(err, pseudosettle.ErrPeerNoSettlements) {
			s.Logger.Debugf("debug api: settlements peer: get peer %s sent settlement: %v", peer.String(), err)
			s.Logger.Errorf("debug api: settlements peer: can't get peer %s sent settlement", peer.String())
			jsonhttp.InternalServerError(w, errCantSettlementsPeer)
			return
		}
	}

	if err == nil {
		peerexists = true
	}

	if !peerexists {
		jsonhttp.NotFound(w, pseudosettle.ErrPeerNoSettlements)
		return
	}

	jsonhttp.OK(w, settlementResponse{
		Peer:               peer.String(),
		SettlementReceived: received,
		SettlementSent:     sent,
	})
}
