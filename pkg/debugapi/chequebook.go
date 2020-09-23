// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var (
	errChequebookBalance  = "cannot get chequebook balance"
	errCantLastChequePeer = "cannot get last cheque for peer"
	errCantLastCheque     = "cannot get last cheque for all peers"
	errUnknownBeneficary  = "unknown beneficiary for peer"
)

type chequebookBalanceResponse struct {
	TotalBalance     *big.Int `json:"totalBalance"`
	AvailableBalance *big.Int `json:"availableBalance"`
}

type chequebookAddressResponse struct {
	Address string `json:"chequebookaddress"`
}

type chequebookLastChequeResponse struct {
	Peer        string   `json:"peer"`
	Beneficiary string   `json:"beneficiary"`
	Chequebook  string   `json:"chequebook"`
	Payout      *big.Int `json:"payout"`
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
		s.Logger.Debugf("debug api: settlements peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: settlements peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	lastcheque, err := s.Swap.LastChequePeer(peer)
	if err != nil {
		if !errors.Is(err, swap.ErrUnknownBeneficary) {
			s.Logger.Debugf("debug api: lastcheque peer: get peer %s last cheque: %v", peer.String(), err)
			s.Logger.Errorf("debug api: settlements peer: can't get peer %s last cheque", peer.String())
			jsonhttp.InternalServerError(w, errCantLastChequePeer)
			return
		}

		jsonhttp.NotFound(w, errUnknownBeneficary)
		return
	}

	jsonhttp.OK(w, chequebookLastChequeResponse{
		Address:     addr,
		Beneficiary: lastcheque.Cheque.Beneficiary.String(),
		Chequebook:  lastcheque.Cheque.Chequebook.String(),
		Payout:      lastcheque.Cheque.CumulativePayout,
	})
}

func (s *server) chequebookAllLastHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, "ok")
	return
}
