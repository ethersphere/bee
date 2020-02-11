package addressbook

import (
	"github.com/ethersphere/bee/pkg/swarm"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type Getter interface {
	Get(overlay swarm.Address) (underlay libp2ppeer.ID)
}

type Putter interface {
	Put(underlay libp2ppeer.ID, overlay swarm.Address) (exists bool)
}
