//go:build !js
// +build !js

package lightnode

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
)

type Container struct {
	base              swarm.Address
	peerMu            sync.Mutex // peerMu guards connectedPeers and disconnectedPeers.
	connectedPeers    *pslice.PSlice
	disconnectedPeers *pslice.PSlice
	metrics           metrics
}

func NewContainer(base swarm.Address) *Container {
	return &Container{
		base:              base,
		connectedPeers:    pslice.New(1, base),
		disconnectedPeers: pslice.New(1, base),
		metrics:           newMetrics(),
	}
}

func (c *Container) Connected(ctx context.Context, peer p2p.Peer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()

	addr := peer.Address
	c.connectedPeers.Add(addr)
	c.disconnectedPeers.Remove(addr)

	c.metrics.CurrentlyConnectedPeers.Set(float64(c.connectedPeers.Length()))
	c.metrics.CurrentlyDisconnectedPeers.Set(float64(c.disconnectedPeers.Length()))
}

func (c *Container) Disconnected(peer p2p.Peer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()

	addr := peer.Address
	if found := c.connectedPeers.Exists(addr); found {
		c.connectedPeers.Remove(addr)
		c.disconnectedPeers.Add(addr)
	}

	c.metrics.CurrentlyConnectedPeers.Set(float64(c.connectedPeers.Length()))
	c.metrics.CurrentlyDisconnectedPeers.Set(float64(c.disconnectedPeers.Length()))
}
