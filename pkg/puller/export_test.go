package puller

import "github.com/ethersphere/bee/pkg/swarm"

var (
	PeerIntervalKey = peerIntervalKey
)

func (p *Puller) IsSyncing(addr swarm.Address) bool {
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	for _, bin := range p.syncPeers {
		for peer := range bin {
			if addr.ByteString() == peer {
				return true
			}
		}
	}
	return false
}
