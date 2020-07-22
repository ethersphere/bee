package settlement

import "github.com/ethersphere/bee/pkg/swarm"

type Interface interface {
	Pay(peer swarm.Address, amount uint64) error
}

type PaymentObserver interface {
	NotifyPayment(peer swarm.Address, amount uint64) error
}
