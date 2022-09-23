// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

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
	str := "/" + mux.Vars(r)["multi-address"]
	addr, err := multiaddr.NewMultiaddr(str)
	if err != nil {
		s.logger.Debug("peer connect: parse multiaddress string failed", "string", str, "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	bzzAddr, err := s.p2p.Connect(r.Context(), addr)
	if err != nil {
		s.logger.Debug("peer connect: p2p connect failed", "addresses", addr, "error", err)
		s.logger.Error(nil, "peer connect: p2p connect failed", "addresses", addr)
		jsonhttp.InternalServerError(w, err)
		return
	}

	if err := s.topologyDriver.Connected(r.Context(), p2p.Peer{Address: bzzAddr.Overlay}, true); err != nil {
		_ = s.p2p.Disconnect(bzzAddr.Overlay, "failed to notify topology")
		s.logger.Debug("peer connect: connect to peer failed", "addresses", addr, "error", err)
		s.logger.Error(nil, "peer connect: connect to peer failed", "addresses", addr)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, peerConnectResponse{
		Address: bzzAddr.Overlay.String(),
	})
}

func (s *Service) peerDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Address []byte `parse:"address,addressToBytes" name:"address" errMessage:"invalid peer address"`
	}{}
	err := s.parseAndValidate(mux.Vars(r), &path)
	if err != nil {
		s.logger.Debug("peer disconnect: parse address string failed", "string", mux.Vars(r)["address"], "error", err)
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	swarmAdr := swarm.NewAddress(path.Address)
	if err := s.p2p.Disconnect(swarm.NewAddress(path.Address), "user requested disconnect"); err != nil {
		s.logger.Debug("peer disconnect: p2p disconnect failed", "peer_address", swarmAdr, "error", err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.BadRequest(w, "peer not found")
			return
		}
		s.logger.Error(nil, "peer disconnect: p2p disconnect failed", "peer_address", swarmAdr)
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
		s.logger.Debug("blocklisted peers: get blocklisted peers failed", "error", err)
		jsonhttp.InternalServerError(w, "get blocklisted peers failed")
		return
	}

	jsonhttp.OK(w, peersResponse{
		Peers: mapPeers(peers),
	})
}

func mapPeers(peers []p2p.Peer) (out []Peer) {
	out = make([]Peer, 0, len(peers))
	for _, peer := range peers {
		out = append(out, Peer{
			Address:  peer.Address,
			FullNode: peer.FullNode,
		})
	}
	return
}
