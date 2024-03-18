// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
)

type peerConnectResponse struct {
	Address string `json:"address"`
}

func (s *Service) peerConnectHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithValues("post_connect").Build()

	mux.Vars(r)["multi-address"] = "/" + mux.Vars(r)["multi-address"]
	paths := struct {
		MultiAddress multiaddr.Multiaddr `map:"multi-address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	bzzAddr, err := s.p2p.Connect(r.Context(), paths.MultiAddress)
	if err != nil {
		logger.Debug("p2p connect failed", "addresses", paths.MultiAddress, "error", err)
		logger.Error(nil, "p2p connect failed", "addresses", paths.MultiAddress)
		jsonhttp.InternalServerError(w, err)
		return
	}

	if err := s.topologyDriver.Connected(r.Context(), p2p.Peer{Address: bzzAddr.Overlay}, true); err != nil {
		_ = s.p2p.Disconnect(bzzAddr.Overlay, "failed to notify topology")
		logger.Debug("connect to peer failed", "addresses", paths.MultiAddress, "error", err)
		logger.Error(nil, "connect to peer failed", "addresses", paths.MultiAddress)
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, peerConnectResponse{
		Address: bzzAddr.Overlay.String(),
	})
}

func (s *Service) peerDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithValues("delete_peer").Build()

	paths := struct {
		Address swarm.Address `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	if err := s.p2p.Disconnect(paths.Address, "user requested disconnect"); err != nil {
		logger.Debug("p2p disconnect failed", "peer_address", paths.Address, "error", err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}
		logger.Error(nil, "p2p disconnect failed", "peer_address", paths.Address)
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

type BlockListedPeer struct {
	Peer
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

type peersResponse struct {
	Peers []Peer `json:"peers"`
}

type blockListedPeersResponse struct {
	Peers []BlockListedPeer `json:"peers"`
}

func (s *Service) peersHandler(w http.ResponseWriter, _ *http.Request) {
	jsonhttp.OK(w, peersResponse{
		Peers: mapPeers(s.p2p.Peers()),
	})
}

func (s *Service) blocklistedPeersHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithValues("get_blocklist").Build()

	peers, err := s.p2p.BlocklistedPeers()
	if err != nil {
		logger.Debug("get blocklisted peers failed", "error", err)
		jsonhttp.InternalServerError(w, "get blocklisted peers failed")
		return
	}

	jsonhttp.OK(w, blockListedPeersResponse{
		Peers: mapBlockListedPeers(peers),
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

func mapBlockListedPeers(peers []p2p.BlockListedPeer) []BlockListedPeer {
	out := make([]BlockListedPeer, 0, len(peers))
	for _, peer := range peers {
		out = append(out, BlockListedPeer{
			Peer: Peer{
				Address:  peer.Address,
				FullNode: peer.FullNode,
			},
			Reason:   peer.Reason,
			Duration: int(peer.Duration.Seconds()),
		})
	}
	return out
}
