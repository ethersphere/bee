// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import "github.com/ethersphere/bee/v2/pkg/log"

func (m *GSOCEphemeralMode) FormatBroadcast(sub *brokerSubscriber, rawMsg []byte) []byte {
	return m.formatBroadcast(sub, rawMsg)
}

func (m *GSOCEphemeralMode) SetGsocParams(gsocOwner, gsocID []byte) {
	m.setGsocParams(gsocOwner, gsocID)
}

func (m *GSOCEphemeralMode) GsocOwner() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gsocOwner
}

func NewBrokerSubscriber() *brokerSubscriber {
	return &brokerSubscriber{}
}

func (b *brokerSubscriber) SetHandshakeDone() {
	b.handshakeHappened = true
}

func NewTestSubscriberConn(logger log.Logger) *SubscriberConn {
	return &SubscriberConn{
		subs:   make(map[uint64]chan []byte),
		logger: logger,
	}
}

func (sc *SubscriberConn) FanOut(msg []byte) {
	sc.fanOut(msg)
}
