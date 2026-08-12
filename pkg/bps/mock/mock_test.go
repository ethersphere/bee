// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/mock"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestMockOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	want := errors.New("refused")

	svc := mock.New(mock.WithOpenFunc(
		func(context.Context, swarm.Address, *pb.CohortSpec, *pb.PublisherAuth) (bps.Publisher, error) {
			return nil, want
		},
	))

	if _, err := svc.Open(ctx, swarm.ZeroAddress, nil, nil); !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestMockDefaults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := mock.New()

	if _, err := svc.Open(ctx, swarm.ZeroAddress, nil, nil); !errors.Is(err, mock.ErrNotImplemented) {
		t.Fatalf("open: got %v want %v", err, mock.ErrNotImplemented)
	}
	if _, err := svc.Subscribe(ctx, swarm.ZeroAddress, swarm.ZeroAddress, nil); !errors.Is(err, mock.ErrNotImplemented) {
		t.Fatalf("subscribe: got %v want %v", err, mock.ErrNotImplemented)
	}
}
