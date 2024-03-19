// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	mockstorer "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestActAddLookup(t *testing.T) {
	act := dynamicaccess.NewDefaultAct()
	lookupKey := swarm.RandAddress(t).Bytes()
	encryptedAccesskey := swarm.RandAddress(t).Bytes()
	err := act.Add(lookupKey, encryptedAccesskey)
	if err != nil {
		t.Error("Add() should not return an error")
	}

	key, _ := act.Lookup(lookupKey)
	if !bytes.Equal(key, encryptedAccesskey) {
		t.Errorf("Get() value is not the expected %s != %s", key, encryptedAccesskey)
	}
}

func TestActWithManifest(t *testing.T) {

	storer := mockstorer.New()
	encrypt := false
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false, 0))
	rootManifest, err := manifest.NewDefaultManifest(ls, encrypt)
	if err != nil {
		t.Error("DefaultManifest should not return an error")
	}

	act := dynamicaccess.NewDefaultAct()
	lookupKey := swarm.RandAddress(t).Bytes()
	encryptedAccesskey := swarm.RandAddress(t).Bytes()
	err = act.Add(lookupKey, encryptedAccesskey)
	if err != nil {
		t.Error("Add() should not return an error")
	}

	actManifEntry, _ := act.Load(lookupKey)
	if actManifEntry == nil {
		t.Error("Load() should return a manifest.Entry")
	}

	err = rootManifest.Add(ctx, hex.EncodeToString(lookupKey), actManifEntry)
	if err != nil {
		t.Error("rootManifest.Add() should not return an error")
	}

	_, err = rootManifest.Store(ctx)
	if err != nil {
		t.Error("rootManifest.Store() should not return an error")
	}

	actualMe, err := rootManifest.Lookup(ctx, hex.EncodeToString(lookupKey))
	if err != nil {
		t.Error("rootManifest.Lookup() should not return an error")
	}

	actualAct := dynamicaccess.NewDefaultAct()
	actualAct.Store(actualMe)
	actualEak, _ := actualAct.Lookup(lookupKey)
	if !bytes.Equal(actualEak, encryptedAccesskey) {
		t.Errorf("actualAct.Store() value is not the expected %s != %s", actualEak, encryptedAccesskey)
	}
}

func TestActStore(t *testing.T) {
	mp := make(map[string]string)

	lookupKey := swarm.RandAddress(t).Bytes()
	encryptedAccesskey := swarm.RandAddress(t).Bytes()
	mp[hex.EncodeToString(lookupKey)] = hex.EncodeToString(encryptedAccesskey)

	me := manifest.NewEntry(swarm.NewAddress(lookupKey), mp)
	act := dynamicaccess.NewDefaultAct()
	act.Store(me)
	eak, _ := act.Lookup(lookupKey)

	if !bytes.Equal(eak, encryptedAccesskey) {
		t.Errorf("Store() value is not the expected %s != %s", eak, encryptedAccesskey)
	}

}

func TestActLoad(t *testing.T) {
	act := dynamicaccess.NewDefaultAct()
	lookupKey := swarm.RandAddress(t).Bytes()
	encryptedAccesskey := swarm.RandAddress(t).Bytes()
	act.Add(lookupKey, encryptedAccesskey)
	me, _ := act.Load(lookupKey)

	eak := me.Metadata()[hex.EncodeToString(lookupKey)]

	if eak != hex.EncodeToString(encryptedAccesskey) {
		t.Errorf("Load() value is not the expected %s != %s", eak, encryptedAccesskey)
	}

}

func pipelineFactory(s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(context.Background(), s, encrypt, rLevel)
	}
}
