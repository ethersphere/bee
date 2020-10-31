package postage

import "github.com/ethersphere/bee/pkg/swarm"

func (st *StampIssuer) Inc(a swarm.Address) error {
	return st.inc(a)
}
