// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinning

import (
	"context"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	statestorem "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	storagem "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/traversal"
)

var _ Interface = (*Service)(nil)

func TestPinningService(t *testing.T) {
	const content = "Hello, Bee!"

	var (
		ctx        = context.Background()
		storerMock = storagem.NewStorer()
		service    = NewService(
			storerMock,
			statestorem.NewStateStore(),
			traversal.NewService(storerMock),
		)
	)

	pipe := builder.NewPipelineBuilder(ctx, storerMock, storage.ModePutUpload, false)
	addr, err := builder.FeedPipeline(ctx, pipe, strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("create and list", func(t *testing.T) {
		if err := service.CreatePin(ctx, addr, false); err != nil {
			t.Fatalf("CreatePin(...): unexpected error: %v", err)
		}
		addrs, err := service.Pins()
		if err != nil {
			t.Fatalf("Pins(...): unexpected error: %v", err)
		}
		if have, want := len(addrs), 1; have != want {
			t.Fatalf("Pins(...): have %d; want %d", have, want)
		}
		if have, want := addrs[0], addr; !have.Equal(want) {
			t.Fatalf("address mismatch: have %q; want %q", have, want)
		}
	})

	t.Run("create idempotent and list", func(t *testing.T) {
		if err := service.CreatePin(ctx, addr, false); err != nil {
			t.Fatalf("CreatePin(...): unexpected error: %v", err)
		}
		addrs, err := service.Pins()
		if err != nil {
			t.Fatalf("Pins(...): unexpected error: %v", err)
		}
		if have, want := len(addrs), 1; have != want {
			t.Fatalf("Pins(...): have %d; want %d", have, want)
		}
		if have, want := addrs[0], addr; !have.Equal(want) {
			t.Fatalf("address mismatch: have %q; want %q", have, want)
		}
	})

	t.Run("delete and has", func(t *testing.T) {
		err := service.DeletePin(ctx, addr)
		if err != nil {
			t.Fatalf("DeletePin(...): unexpected error: %v", err)
		}
		has, err := service.HasPin(addr)
		if err != nil {
			t.Fatalf("HasPin(...): unexpected error: %v", err)
		}
		if has {
			t.Fatalf("HasPin(...): have %t; want %t", has, !has)
		}
	})

	t.Run("delete idempotent and has", func(t *testing.T) {
		err := service.DeletePin(ctx, addr)
		if err != nil {
			t.Fatalf("DeletePin(...): unexpected error: %v", err)
		}
		has, err := service.HasPin(addr)
		if err != nil {
			t.Fatalf("HasPin(...): unexpected error: %v", err)
		}
		if has {
			t.Fatalf("HasPin(...): have %t; want %t", has, !has)
		}
	})
}
