// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ swarm.ChunkValidator = (*Validator)(nil)

type Validator struct {
	rv bool
}

// NewValidator constructs a new Validator
func NewValidator(rv bool) swarm.ChunkValidator {
	return &Validator{rv: rv}
}

// Validate returns rv from mock struct
func (v *Validator) Validate(ch swarm.Chunk) (valid bool) {
	return v.rv
}
