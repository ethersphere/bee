package mock

import "github.com/ethersphere/bee/pkg/swarm"

type PeerFinderMock struct {
	found bool
}

func (p *PeerFinderMock) SetFound(found bool) {
	p.found = found
}

func (p *PeerFinderMock) Exists(overlay swarm.Address) (found bool) {
	return p.found
}
