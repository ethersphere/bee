// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/soc"
)

// Binding exposes the unexported binding interface to tests, with exported
// method names.
type Binding interface {
	Qualifies(spec *pb.CohortSpec, s *soc.SOC) error
	DedupKey(s *soc.SOC) ([]byte, error)
}

type exportedBinding struct{ b binding }

func (e exportedBinding) Qualifies(spec *pb.CohortSpec, s *soc.SOC) error {
	return e.b.qualifies(spec, s)
}

func (e exportedBinding) DedupKey(s *soc.SOC) ([]byte, error) {
	return e.b.dedupKey(s)
}

// BindingFor exposes bindingFor to tests.
func BindingFor(b pb.TopicBinding) (Binding, error) {
	bb, err := bindingFor(b)
	if err != nil {
		return nil, err
	}
	return exportedBinding{b: bb}, nil
}

// AuthorizePublisher exposes authorizePublisher to tests.
var AuthorizePublisher = authorizePublisher

// StatusOf exposes statusOf to tests.
var StatusOf = statusOf
