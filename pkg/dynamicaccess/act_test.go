// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
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

func TestActStoreLoad(t *testing.T) {

	act := dynamicaccess.NewDefaultAct()
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

	actualAct := dynamicaccess.NewDefaultAct()
	actualAct.Load(swarm_ref)
	actualEak, _ := actualAct.Lookup(lookupKey)
	if !bytes.Equal(actualEak, encryptedAccesskey) {
		t.Errorf("actualAct.Load() value is not the expected %s != %s", hex.EncodeToString(actualEak), hex.EncodeToString(encryptedAccesskey))
	}
}
