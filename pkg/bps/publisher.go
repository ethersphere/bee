// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
)

// ErrNotPublisher is returned when an owner is outside the cohort's genesis
// publisher set.
var ErrNotPublisher = errors.New("bps: not a legitimate publisher")

// authorizePublisher checks owner against the cohort's publisher regime.
//
// Under explicit regimes the genesis set decides, and it is fixed at Open:
// dynamic grants and revocations are deferred by SWIP-60 to a later revision.
//
// At handshake time the owner is only declared, never proved — a PublisherAuth
// is not a credential. This check is an early refusal. The binding gate is the
// same call at Publish time, against the owner recovered from the message's
// signature, which is what actually authenticates a publisher.
func authorizePublisher(spec *pb.CohortSpec, owner []byte) error {
	if len(owner) == 0 {
		return fmt.Errorf("no owner: %w", ErrNotPublisher)
	}

	switch spec.GetPublishers() {
	case pb.PublisherRegime_EXPLICIT_SINGLE:
		if !bytes.Equal(owner, spec.GetAdmin()) {
			return fmt.Errorf("owner %x is not the admin: %w", owner, ErrNotPublisher)
		}
		return nil
	case pb.PublisherRegime_EXPLICIT_LIST:
		for _, p := range Publishers(spec) {
			if bytes.Equal(owner, p) {
				return nil
			}
		}
		return fmt.Errorf("owner %x is not on the publisher list: %w", owner, ErrNotPublisher)
	default:
		return fmt.Errorf("regime %s: %w", spec.GetPublishers(), ErrUnsupportedRegime)
	}
}
