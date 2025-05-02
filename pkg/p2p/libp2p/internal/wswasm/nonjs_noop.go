//go:build !js
// +build !js

package wswasm

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/transport"
)

type WebsocketTransport struct {
	transport.Transport
	upgrader transport.Upgrader
	rcmgr    network.ResourceManager
}

func New(upgrader transport.Upgrader, rcmgr network.ResourceManager) (*WebsocketTransport, error) {
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	return &WebsocketTransport{
		upgrader: upgrader,
		rcmgr:    rcmgr,
	}, nil
}
