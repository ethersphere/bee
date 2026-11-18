// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/contractversion"
)

type versioned struct {
	s *contractversion.Switch[Contract]
}

var _ Contract = (*versioned)(nil)

// NewVersioned returns a Contract that forwards every call to the version
// active at the current block height.
func NewVersioned(s *contractversion.Switch[Contract]) Contract {
	return &versioned{s: s}
}

func (v *versioned) active(ctx context.Context) (Contract, error) {
	c, err := v.s.Active(ctx)
	if err != nil {
		return nil, fmt.Errorf("active redistribution contract: %w", err)
	}
	return c, nil
}

func (v *versioned) ReserveSalt(ctx context.Context) ([]byte, error) {
	c, err := v.active(ctx)
	if err != nil {
		return nil, err
	}
	return c.ReserveSalt(ctx)
}

func (v *versioned) IsPlaying(ctx context.Context, depth uint8) (bool, error) {
	c, err := v.active(ctx)
	if err != nil {
		return false, err
	}
	return c.IsPlaying(ctx, depth)
}

func (v *versioned) IsWinner(ctx context.Context) (bool, error) {
	c, err := v.active(ctx)
	if err != nil {
		return false, err
	}
	return c.IsWinner(ctx)
}

func (v *versioned) Claim(ctx context.Context, proofs ChunkInclusionProofs) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.Claim(ctx, proofs)
}

func (v *versioned) Commit(ctx context.Context, obfusHash []byte, round uint64) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.Commit(ctx, obfusHash, round)
}

func (v *versioned) Reveal(ctx context.Context, storageDepth uint8, reserveCommitmentHash []byte, RandomNonce []byte) (common.Hash, error) {
	c, err := v.active(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return c.Reveal(ctx, storageDepth, reserveCommitmentHash, RandomNonce)
}
