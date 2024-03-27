// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kvs_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/kvs"
	"github.com/ethersphere/bee/pkg/storage"
	mockstorer "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var mockStorer = mockstorer.New()

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}

func createLs() file.LoadSaver {
	return loadsave.New(mockStorer.ChunkStore(), mockStorer.Cache(), requestPipelineFactory(context.Background(), mockStorer.Cache(), false, redundancy.NONE))
}

func keyValuePair(t *testing.T) ([]byte, []byte) {
	return swarm.RandAddress(t).Bytes(), swarm.RandAddress(t).Bytes()
}

func TestKvsAddLookup(t *testing.T) {
	ls := createLs()

	putter := mockStorer.DirectUpload()
	s := kvs.New(ls, putter, swarm.ZeroAddress)

	lookupKey, encryptedAccesskey := keyValuePair(t)

	err := s.Put(lookupKey, encryptedAccesskey)
	if err != nil {
		t.Errorf("Add() should not return an error: %v", err)
	}

	key, err := s.Get(lookupKey)
	if err != nil {
		t.Errorf("Lookup() should not return an error: %v", err)
	}

	if !bytes.Equal(key, encryptedAccesskey) {
		t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(key), hex.EncodeToString(encryptedAccesskey))
	}

}

func TestKvsAddLookupWithSave(t *testing.T) {
	ls := createLs()
	putter := mockStorer.DirectUpload()
	s1 := kvs.New(ls, putter, swarm.ZeroAddress)
	lookupKey, encryptedAccesskey := keyValuePair(t)

	err := s1.Put(lookupKey, encryptedAccesskey)
	if err != nil {
		t.Fatalf("Add() should not return an error: %v", err)
	}
	ref, err := s1.Save()
	if err != nil {
		t.Fatalf("Save() should not return an error: %v", err)
	}
	s2 := kvs.New(ls, putter, ref)
	key, err := s2.Get(lookupKey)
	if err != nil {
		t.Fatalf("Lookup() should not return an error: %v", err)
	}

	if !bytes.Equal(key, encryptedAccesskey) {
		t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(key), hex.EncodeToString(encryptedAccesskey))
	}

}

func TestKvsAddSaveAdd(t *testing.T) {
	ls := createLs()
	putter := mockStorer.DirectUpload()
	kvs1 := kvs.New(ls, putter, swarm.ZeroAddress)
	kvs1key1, kvs1val1 := keyValuePair(t)

	err := kvs1.Put(kvs1key1, kvs1val1)
	if err != nil {
		t.Fatalf("Add() should not return an error: %v", err)
	}
	ref, err := kvs1.Save()
	if err != nil {
		t.Fatalf("Save() should not return an error: %v", err)
	}

	// New KVS
	kvs2 := kvs.New(ls, putter, ref)

	kvs2key1, kvs2val1 := keyValuePair(t)

	// put after save
	kvs2.Put(kvs2key1, kvs2val1)

	// get after save
	kvs2get1, err := kvs2.Get(kvs2key1)
	if err != nil {
		t.Fatalf("Lookup() should not return an error: %v", err)
	}
	if !bytes.Equal(kvs2get1, kvs2val1) {
		t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(kvs2key1), hex.EncodeToString(kvs2val1))
	}

	// get before Save
	kvs2get2, err := kvs2.Get(kvs1key1)

	if err != nil {
		t.Fatalf("Lookup() should not return an error: %v", err)
	}
	if !bytes.Equal(kvs2get2, kvs1val1) {
		t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(kvs2get2), hex.EncodeToString(kvs1val1))
	}

}
