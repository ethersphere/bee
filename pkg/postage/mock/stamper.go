// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type mockStamper struct{}

func NewStamper() postage.Stamper {
	return &mockStamper{}
}

func (mockStamper) Stamp(_ swarm.Address) (*postage.Stamp, error) {
	return &postage.Stamp{}, nil
}
