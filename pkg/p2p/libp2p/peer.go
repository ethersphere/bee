package libp2p

import "github.com/libp2p/go-libp2p-core/peer"

type Peer struct {
	overlay string
	peerID peer.ID
}

func (p *Peer) Overlay() string {
	return p.Overlay()
}
