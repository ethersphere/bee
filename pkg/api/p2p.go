// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/multiformats/go-multiaddr"
)

type addressesResponse struct {
	Overlay      *swarm.Address        `json:"overlay"`
	Underlay     []multiaddr.Multiaddr `json:"underlay"`
	Ethereum     common.Address        `json:"ethereum"` // deprecated
	ChainAddress common.Address        `json:"chain_address"`
	PublicKey    string                `json:"publicKey"`
	PSSPublicKey string                `json:"pssPublicKey"`
}

func (s *Service) addressesHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithValues("get_addresses").Build()

	// initialize variable to json encode as [] instead null if p2p is nil
	underlay := make([]multiaddr.Multiaddr, 0)
	// addresses endpoint is exposed before p2p service is configured
	// to provide information about other addresses.
	if s.p2p != nil {
		u, err := s.p2p.Addresses()
		if err != nil {
			logger.Debug("get address failed", "error", err)
			jsonhttp.InternalServerError(w, err)
			return
		}
		underlay = u
	}
	jsonhttp.OK(w, addressesResponse{
		Overlay:      s.overlay,
		Underlay:     underlay,
		Ethereum:     s.ethereumAddress,
		ChainAddress: s.ethereumAddress,
		PublicKey:    hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&s.publicKey)),
		PSSPublicKey: hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&s.pssPublicKey)),
	})
}
