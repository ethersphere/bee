// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
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

func (m *MockPusher) SendChunk(uid uint32) error {
	ta, err := m.tag.Get(uid)
	if err != nil {
		return err
	}
	return ta.Inc(tags.StateSent)
}

func (m *MockPusher) RcvdReceipt(uid uint32) error {
	ta, err := m.tag.Get(uid)
	if err != nil {
		return err
	}
	return ta.Inc(tags.StateSynced)
}
