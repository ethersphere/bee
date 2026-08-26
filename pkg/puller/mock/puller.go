// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import "context"

type Option func(*mockSyncer)

type mockSyncer struct {
	rate                float64
	isReserveSyncedFunc func(depth uint8) bool
	isBinSyncingFunc    func(bin uint8) bool
}

func WithReserveSynced(f func(depth uint8) bool) Option {
	return func(m *mockSyncer) {
		m.isReserveSyncedFunc = f
	}
}

func WithBinSyncing(f func(bin uint8) bool) Option {
	return func(m *mockSyncer) {
		m.isBinSyncingFunc = f
	}
}

func NewMockRateReporter(r float64, opts ...Option) *mockSyncer {
	m := &mockSyncer{rate: r}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *mockSyncer) SyncRate() float64 { return m.rate }

func (m *mockSyncer) IsReserveSynced(depth uint8) bool {
	if m.isReserveSyncedFunc != nil {
		return m.isReserveSyncedFunc(depth)
	}
	return true
}

func (m *mockSyncer) IsBinSyncing(bin uint8) bool {
	if m.isBinSyncingFunc != nil {
		return m.isBinSyncingFunc(bin)
	}
	return false
}

func (m *mockSyncer) Start(context.Context) {}
