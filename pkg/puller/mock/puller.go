// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import "context"

type mockSyncer struct{ synced bool }

func NewMockSyncer(synced bool) *mockSyncer      { return &mockSyncer{synced} }
func (m *mockSyncer) IsReserveSynced(uint8) bool { return m.synced }
func (m *mockSyncer) Start(context.Context)      {}
