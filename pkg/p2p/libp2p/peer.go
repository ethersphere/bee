package libp2p

import libp2ppeer "github.com/libp2p/go-libp2p-core/peer"

type peer struct {
	overlay string
	peerID  libp2ppeer.ID
}

