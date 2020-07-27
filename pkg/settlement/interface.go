package settlement

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
)

type Interface interface {
	Pay(ctx context.Context, peer swarm.Address, amount uint64) error
}

type PaymentObserver interface {
	NotifyPayment(peer swarm.Address, amount uint64) error
}
