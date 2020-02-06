package libp2p

import (
	"context"

	"github.com/ethersphere/bee/pkg/p2p"
)

type ProtocolService interface {
	Init(ctx context.Context, peer p2p.Peer) error
	Start()
}
