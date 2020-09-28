// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"encoding/hex"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/multiformats/go-multiaddr"
)

type addressesResponse struct {
	Overlay   swarm.Address         `json:"overlay"`
	Underlay  []multiaddr.Multiaddr `json:"underlay"`
	Ethereum  common.Address        `json:"ethereum"`
	PublicKey string                `json:"public_key"`
}

func (s *server) addressesHandler(w http.ResponseWriter, r *http.Request) {
	underlay, err := s.P2P.Addresses()
	if err != nil {
		s.Logger.Debugf("debug api: p2p addresses: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, addressesResponse{
		Overlay:   s.Overlay,
		Underlay:  underlay,
		Ethereum:  s.EthereumAddress,
		PublicKey: hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&s.PublicKey)),
	})
}
