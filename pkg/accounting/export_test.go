package accounting

import (
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

func (s *Accounting) SetTimeNow(f func() time.Time) {
	s.timeNow = f
}

func (s *Accounting) SetTime(k int64) {
	s.SetTimeNow(func() time.Time {
		return time.Unix(k, 0)
	})
}

func (a *Accounting) IsPaymentOngoing(peer swarm.Address) bool {
	return a.getAccountingPeer(peer).paymentOngoing
}
