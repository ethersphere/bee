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

type chequebookLastChequePeerResponse struct {
	Address     string   `json:"address"`
	Beneficiary string   `json:"beneficiary"`
	Chequebook  string   `json:"chequebook"`
	Payout      *big.Int `json:"payout"`
}

type chequebookLastChequesPeerResponse struct {
	LastChequeIn  chequebookLastChequePeerResponse `json:"lastchequein"`
	LastChequeOut chequebookLastChequePeerResponse `json:"lastchequeout"`
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
		s.Logger.Debugf("debug api: chequebook lastcheque peer: invalid peer address %s: %v", addr, err)
		s.Logger.Error("debug api: chequebook lastcheque peer: invalid peer address %s", addr)
		jsonhttp.NotFound(w, errInvaliAddress)
		return
	}

	lastchequeIn, err := s.Swap.LastChequePeer(peer)
	lastchequeOut, err := s.Swap.LastStoredChequePeer(peer)
	if err != nil {
		if !errors.Is(err, swap.ErrUnknownBeneficary) {
			s.Logger.Debugf("debug api: chequebook lastcheque peer: get peer %s last cheque: %v", peer.String(), err)
			s.Logger.Errorf("debug api: chequebook lastcheque peer: can't get peer %s last cheque", peer.String())
			jsonhttp.InternalServerError(w, errCantLastChequePeer)
			return
		}

		jsonhttp.NotFound(w, errUnknownBeneficary)
		return
	}

	lastin := chequebookLastChequePeerResponse{
		Address:     addr,
		Beneficiary: lastchequeIn.Cheque.Beneficiary.String(),
		Chequebook:  lastchequeIn.Cheque.Chequebook.String(),
		Payout:      lastchequeIn.Cheque.CumulativePayout,
	}

	lastout := chequebookLastChequePeerResponse{
		Address:     addr,
		Beneficiary: lastchequeOut.Cheque.Beneficiary.String(),
		Chequebook:  lastchequeOut.Cheque.Chequebook.String(),
		Payout:      lastchequeOut.Cheque.CumulativePayout,
	}

	jsonhttp.OK(w, chequebookLastChequesPeerResponse{
		LastChequeIn:  lastin,
		LastChequeOut: lastout,
	})
}

func (s *server) chequebookAllLastHandler(w http.ResponseWriter, r *http.Request) {

	lastcheques, err := s.Swap.LastCheques()
	laststoredcheques, err := s.Swap.LastStoredCheques()

	if err != nil {
		jsonhttp.InternalServerError(w, errCantLastCheque)
	}

	lcr := make(map[string]chequebookLastChequesPeerResponse)

	for i, j := range lastcheques {
		lcr[i] = chequebookLastChequesPeerResponse{
			LastChequeIn: chequebookLastChequePeerResponse{
				Address:     i,
				Beneficiary: j.Cheque.Beneficiary.String(),
				Chequebook:  j.Cheque.Chequebook.String(),
				Payout:      j.Cheque.CumulativePayout,
			},
			LastChequeOut: chequebookLastChequePeerResponse{},
		}
	}

	for i, j := range laststoredcheques {
		if _, ok := lcr[i]; ok {
			t := lcr[i]
			t.LastChequeOut = chequebookLastChequePeerResponse{
				Address:     i,
				Beneficiary: j.Cheque.Beneficiary.String(),
				Chequebook:  j.Cheque.Chequebook.String(),
				Payout:      j.Cheque.CumulativePayout,
			}
			lcr[i] = t
		} else {
			lcr[i] = chequebookLastChequesPeerResponse{
				LastChequeIn: chequebookLastChequePeerResponse{},
				LastChequeOut: chequebookLastChequePeerResponse{
					Address:     i,
					Beneficiary: j.Cheque.Beneficiary.String(),
					Chequebook:  j.Cheque.Chequebook.String(),
					Payout:      j.Cheque.CumulativePayout,
				},
			}
		}

		lcresponses := make([]chequebookLastChequesPeerResponse, len(lcr))
		i := 0
		for k := range lcr {
			lcresponses[i] = lcr[k]
			i++
		}

		jsonhttp.OK(w, chequebookLastChequesResponse{LastCheques: lcresponses})
		return
	}
}
