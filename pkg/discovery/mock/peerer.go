package mock

import (
	"github.com/ethersphere/bee/pkg/p2p"
)

type peersVal = []p2p.Peer
type peersMap = map[int]peersVal

type Peerer struct {
	peers map[string]peersMap
}

func NewPeerer() *Peerer {
	return &Peerer{
		peers: make(map[string]peersMap),
	}
}

func (p *Peerer) Peers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer) {
	peers = p.peers[peer.Address.String()][bin]

	if limit != 0 {
		if limit > len(peers) {
			limit = len(peers)
		}

		peers = peers[0:limit]
	}

	return peers
}

func (p *Peerer) Add(peer p2p.Peer, bin int, peers ...p2p.Peer) {
	peersMap, ok := p.peers[peer.Address.String()]
	if !ok {
		peersMap = make(map[int]peersVal)
	}

	peersMap[bin] = append(peersMap[bin], peers...)
	p.peers[peer.Address.String()] = peersMap
}
