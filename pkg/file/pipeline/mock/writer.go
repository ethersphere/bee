// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"sync"

	"github.com/ethersphere/bee/pkg/file/pipeline"
)

type MockChainWriter struct {
	sync.Mutex
	chainWriteCalls int
	sumCalls        int
}

func NewChainWriter() *MockChainWriter {
	return &MockChainWriter{}
}

func (c *MockChainWriter) ChainWrite(_ *pipeline.PipeWriteArgs) error {
	c.Lock()
	defer c.Unlock()
	c.chainWriteCalls++
	return nil
}
func (c *MockChainWriter) Sum() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	c.sumCalls++
	return nil, nil
}

func (c *MockChainWriter) ChainWriteCalls() int { c.Lock(); defer c.Unlock(); return c.chainWriteCalls }
func (c *MockChainWriter) SumCalls() int        { c.Lock(); defer c.Unlock(); return c.sumCalls }
