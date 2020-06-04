// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

type MockPusher struct {
	tag *tags.Tags
}

func NewMockPusher(tag *tags.Tags) *MockPusher {
	return &MockPusher{
		tag: tag,
	}
}

func (m *MockPusher) SendChunk(address swarm.Address) error {
	ta, err := m.tag.GetByAddress(address)
	if err != nil {
		return err
	}
	ta.Inc(tags.StateSent)

	return nil
}

func (m *MockPusher) RcvdReceipt(address swarm.Address) error {
	ta, err := m.tag.GetByAddress(address)
	if err != nil {
		return err
	}
	ta.Inc(tags.StateSynced)

	return nil
}
