// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
)

type peerConnectResponse struct {
	Address string `json:"address"`
}

func (s *Service) peerConnectHandler(w http.ResponseWriter, r *http.Request) {
	addr, err := multiaddr.NewMultiaddr("/" + mux.Vars(r)["multi-address"])
	if err != nil {
		s.logger.Debugf("debug api: peer connect: parse multiaddress: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	bzzAddr, err := s.p2p.Connect(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("debug api: peer connect %s: %v", addr, err)
		s.logger.Errorf("unable to connect to peer %s", addr)
		jsonhttp.InternalServerError(w, err)
		return
	}

	if err := s.topologyDriver.Connected(r.Context(), p2p.Peer{Address: bzzAddr.Overlay}, true); err != nil {
		_ = s.p2p.Disconnect(bzzAddr.Overlay)
		s.logger.Debugf("debug api: peer connect handler %s: %v", addr, err)
		s.logger.Errorf("unable to connect to peer %s", addr)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, peerConnectResponse{
		Address: bzzAddr.Overlay.String(),
	})
}

func (s *Service) peerDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["address"]
	swarmAddr, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.logger.Debugf("debug api: parse peer address %s: %v", addr, err)
		jsonhttp.BadRequest(w, "invalid peer address")
		return
	}

	if err := s.p2p.Disconnect(swarmAddr); err != nil {
		s.logger.Debugf("debug api: peer disconnect %s: %v", addr, err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.BadRequest(w, "peer not found")
			return
		}
		s.logger.Errorf("unable to disconnect peer %s", addr)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, nil)
}

// Peer holds information about a Peer.
type Peer struct {
	Address  swarm.Address `json:"address"`
	FullNode bool          `json:"fullNode"`
}

type peersResponse struct {
	Peers []Peer `json:"peers"`
}

func (s *Service) peersHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, peersResponse{
		Peers: mapPeers(s.p2p.Peers()),
	})
}

func (s *Service) blocklistedPeersHandler(w http.ResponseWriter, r *http.Request) {
	peers, err := s.p2p.BlocklistedPeers()
	if err != nil {
		s.logger.Debugf("debug api: blocklisted peers: %v", err)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, peersResponse{
		Peers: mapPeers(peers),
	})
}

func mapPeers(peers []p2p.Peer) (out []Peer) {
	for _, peer := range peers {
		out = append(out, Peer{
			Address:  peer.Address,
			FullNode: peer.FullNode,
		})
	}
	return
}
