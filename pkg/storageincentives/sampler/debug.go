// this file together with rchash endpoint and api support should be removed

package sampler

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
)

type SampleWithProofs struct {
	Hash     swarm.Address          `json:"hash"`
	Proofs   []redistribution.Proof `json:"proofs"`
	Duration time.Duration          `json:"duration"`
}

// ReserveWithProofs is only called by rchash API
func (s *Sampler) ReserveSampleWithProofs(ctx context.Context, anchor1, anchor2 []byte, depth uint8, round uint64) (swp SampleWithProofs, err error) {
	t := time.Now()

	sample, err := s.MakeSample(ctx, anchor1, depth, round)
	if err != nil {
		return swp, fmt.Errorf("mock reserve sampling: %w", err)
	}

	proofs, err := MakeProofs(sample, anchor2)
	if err != nil {
		return swp, fmt.Errorf("make proofs: %w", err)
	}

	return SampleWithProofs{sample.Hash, proofs, time.Since(t)}, nil
}
