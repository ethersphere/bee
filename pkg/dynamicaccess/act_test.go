// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
	mockstorer "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ActType int

const (
	Memory ActType = iota
	Manifest
)

func (a ActType) String() string {
	return [...]string{"Memory", "Manifest"}[a]
}

var mockStorer = mockstorer.New()

var acts = map[ActType]func() dynamicaccess.Act{
	Memory:   dynamicaccess.NewInMemoryAct,
	Manifest: func() dynamicaccess.Act { return dynamicaccess.NewInManifestAct(mockStorer) },
}

func TestActAddLookup(t *testing.T) {
	for actType, actCreate := range acts {
		t.Logf("Running test for ActType: %s", actType)
		act := actCreate()

		lookupKey := swarm.RandAddress(t).Bytes()
		encryptedAccesskey := swarm.RandAddress(t).Bytes()

		ref, err := act.Add(swarm.EmptyAddress, lookupKey, encryptedAccesskey)
		if err != nil {
			t.Errorf("Add() should not return an error: %v", err)
		}

		key, err := act.Lookup(ref, lookupKey)
		if err != nil {
			t.Errorf("Lookup() should not return an error: %v", err)
		}

		if !bytes.Equal(key, encryptedAccesskey) {
			t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(key), hex.EncodeToString(encryptedAccesskey))
		}
	}
}

func TestActAddLookupWithNew(t *testing.T) {
	for actType, actCreate := range acts {
		t.Logf("Running test for ActType: %s", actType)
		act1 := actCreate()
		lookupKey := swarm.RandAddress(t).Bytes()
		encryptedAccesskey := swarm.RandAddress(t).Bytes()

		ref, err := act1.Add(swarm.EmptyAddress, lookupKey, encryptedAccesskey)
		if err != nil {
			t.Fatalf("Add() should not return an error: %v", err)
		}

		act2 := actCreate()
		key, err := act2.Lookup(ref, lookupKey)
		if err != nil {
			t.Fatalf("Lookup() should not return an error: %v", err)
		}

		if !bytes.Equal(key, encryptedAccesskey) {
			t.Errorf("Get() value is not the expected %s != %s", hex.EncodeToString(key), hex.EncodeToString(encryptedAccesskey))
		}
	}
}

/*
func TestActStoreLoad(t *testing.T) {

	act := dynamicaccess.NewInMemoryAct()
	lookupKey := swarm.RandAddress(t).Bytes()
	encryptedAccesskey := swarm.RandAddress(t).Bytes()
	err := act.Add(lookupKey, encryptedAccesskey)
	if err != nil {
		t.Error("Add() should not return an error")
	}

	swarm_ref, err := act.Store()
	if err != nil {
		t.Error("Store() should not return an error")
	}

	actualAct := dynamicaccess.NewInMemoryAct()
	actualAct.Load(swarm_ref)
	actualEak, _ := actualAct.Lookup(lookupKey)
	if !bytes.Equal(actualEak, encryptedAccesskey) {
		t.Errorf("actualAct.Load() value is not the expected %s != %s", hex.EncodeToString(actualEak), hex.EncodeToString(encryptedAccesskey))
	}
}
*/
