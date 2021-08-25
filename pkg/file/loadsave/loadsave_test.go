// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loadsave_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	data    = []byte{0, 1, 2, 3}
	expHash = "4f7e85bb4282fd468a9ce4e6e50b6c4b8e6a34aa33332b604c83fb9b2e55978a"
)

func TestLoadSave(t *testing.T) {
	store := mock.NewStorer()
	ls := loadsave.New(store, pipelineFn(store))
	ref, err := ls.Save(context.Background(), data)

	if err != nil {
		t.Fatal(err)
	}
	if r := hex.EncodeToString(ref); r != expHash {
		t.Fatalf("expected hash %s got %s", expHash, r)
	}
	b, err := ls.Load(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, b) {
		t.Fatal("wrong data in response")
	}
}

func TestReadonlyLoadSave(t *testing.T) {
	store := mock.NewStorer()
	factory := pipelineFn(store)
	ls := loadsave.NewReadonly(store)
	_, err := ls.Save(context.Background(), data)
	if !errors.Is(err, loadsave.ReadonlyLoadSaveError) {
		t.Fatal("expected error but got none")
	}

	_, err = builder.FeedPipeline(context.Background(), factory(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	b, err := ls.Load(context.Background(), swarm.MustParseHexAddress(expHash).Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, b) {
		t.Fatal("wrong data in response")
	}
}

func pipelineFn(s storage.Storer) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, storage.ModePutRequest, false)
	}
}
