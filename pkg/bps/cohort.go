// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// CohortSpec is the set of genesis parameters that fully describes a cohort.
// It is fixed by the opener and immutable for the cohort's lifetime.
type CohortSpec = pb.CohortSpec

// PoMin is the proximity constraint for implicit topic bindings. It is a
// protocol constant rather than a cohort parameter: an unset proto3 uint32 is
// indistinguishable from 0, which would silently disable the constraint.
// Unused until the implicit bindings are implemented.
const PoMin = 16

var (
	// ErrInvalidSpec is returned for a cohort spec that is structurally invalid.
	ErrInvalidSpec = errors.New("bps: invalid cohort spec")
	// ErrUnsupportedBinding is returned for a topic binding this implementation
	// does not yet serve.
	ErrUnsupportedBinding = errors.New("bps: unsupported topic binding")
	// ErrUnsupportedRegime is returned for a publisher regime this
	// implementation does not yet serve.
	ErrUnsupportedRegime = errors.New("bps: unsupported publisher regime")
)

// ValidateSpec checks a cohort spec for structural validity and for features
// this implementation supports. Enum zero values are invalid on the wire, so
// an unset binding or regime is an error, never a default.
func ValidateSpec(spec *pb.CohortSpec) error {
	if spec == nil {
		return fmt.Errorf("nil spec: %w", ErrInvalidSpec)
	}
	if len(spec.GetTopic()) != swarm.HashSize {
		return fmt.Errorf("topic length %d: %w", len(spec.GetTopic()), ErrInvalidSpec)
	}

	switch spec.GetBinding() {
	case pb.TopicBinding_ANCHOR, pb.TopicBinding_FEED_TOPIC:
	case pb.TopicBinding_TOPIC_BINDING_UNSPECIFIED:
		return fmt.Errorf("unset binding: %w", ErrInvalidSpec)
	case pb.TopicBinding_SOC_ID, pb.TopicBinding_OWNER:
		return fmt.Errorf("binding %s: %w", spec.GetBinding(), ErrUnsupportedBinding)
	default:
		return fmt.Errorf("binding %d: %w", spec.GetBinding(), ErrInvalidSpec)
	}

	switch spec.GetPublishers() {
	case pb.PublisherRegime_EXPLICIT_SINGLE:
		if len(spec.GetPublisherList()) != 0 {
			return fmt.Errorf("publisher list set under explicit single: %w", ErrInvalidSpec)
		}
	case pb.PublisherRegime_EXPLICIT_LIST:
	case pb.PublisherRegime_PUBLISHER_REGIME_UNSPECIFIED:
		return fmt.Errorf("unset publisher regime: %w", ErrInvalidSpec)
	case pb.PublisherRegime_IMPLICIT, pb.PublisherRegime_ALL:
		return fmt.Errorf("regime %s: %w", spec.GetPublishers(), ErrUnsupportedRegime)
	default:
		return fmt.Errorf("regime %d: %w", spec.GetPublishers(), ErrInvalidSpec)
	}

	// Both supported regimes are explicit, so an admin is mandatory.
	if len(spec.GetAdmin()) != crypto.AddressSize {
		return fmt.Errorf("admin length %d: %w", len(spec.GetAdmin()), ErrInvalidSpec)
	}
	for i, p := range spec.GetPublisherList() {
		if len(p) != crypto.AddressSize {
			return fmt.Errorf("publisher %d length %d: %w", i, len(p), ErrInvalidSpec)
		}
	}

	if spec.GetHistory() {
		return fmt.Errorf("history delivery: %w", ErrInvalidSpec)
	}

	return nil
}

// SpecEqual reports whether two cohort specs describe the same cohort. The
// publisher list is compared as a set: two clients assembling a cohort from the
// same invite may order it differently, and SWIP-60's idempotent Open must
// treat those as identical.
func SpecEqual(a, b *pb.CohortSpec) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if !bytes.Equal(a.GetTopic(), b.GetTopic()) ||
		a.GetBinding() != b.GetBinding() ||
		a.GetPublishers() != b.GetPublishers() ||
		a.GetHistory() != b.GetHistory() ||
		a.GetClosed() != b.GetClosed() ||
		!bytes.Equal(a.GetAdmin(), b.GetAdmin()) ||
		len(a.GetPublisherList()) != len(b.GetPublisherList()) {
		return false
	}

	as := sortedCopy(a.GetPublisherList())
	bs := sortedCopy(b.GetPublisherList())
	for i := range as {
		if !bytes.Equal(as[i], bs[i]) {
			return false
		}
	}
	return true
}

// Publishers returns the cohort's genesis publisher set: the admin followed by
// the publisher list. Meaningful only under explicit regimes.
func Publishers(spec *pb.CohortSpec) [][]byte {
	if spec == nil || len(spec.GetAdmin()) == 0 {
		return nil
	}
	out := make([][]byte, 0, 1+len(spec.GetPublisherList()))
	out = append(out, spec.GetAdmin())
	out = append(out, spec.GetPublisherList()...)
	return out
}

func sortedCopy(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i], out[j]) < 0 })
	return out
}
