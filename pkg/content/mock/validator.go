// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ swarm.Validator = (*ContentAddressValidator)(nil)

type ContentAddressValidator struct {
	rv bool
}

// NewValidator constructs a new Validator
func NewContentAddressValidator(rv bool) swarm.Validator {
	return &ContentAddressValidator{rv: rv}
}

// Validate returns rv from mock struct
func (v *ContentAddressValidator) Validate(ch swarm.Chunk) (valid bool) {
	return v.rv
}
