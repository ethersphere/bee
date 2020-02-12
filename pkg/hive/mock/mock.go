package mock

import (
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ConnectionManagerMock struct {
	Err error
}

func (c ConnectionManagerMock) Connect(underlay []byte) error {
	return c.Err
}

type PeerSuggesterMock struct {
	Peers map[int][]p2p.Peer
}

func (p PeerSuggesterMock) SuggestPeers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer) {
	return p.Peers[bin]
}

type AddressFinderMock struct {
	Underlays map[string][]byte
	Err       error
}

func (a AddressFinderMock) Underlay(overlay swarm.Address) (underlay []byte, err error) {
	if a.Err != nil {
		return nil, a.Err
	}

	return a.Underlays[overlay.String()], nil
}
