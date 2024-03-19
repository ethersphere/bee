// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/manifest"
)

type ActMock struct {
	AddFunc    func(key []byte, val []byte) error
	LookupFunc func(key []byte) ([]byte, error)
	LoadFunc   func(key []byte) (manifest.Entry, error)
	StoreFunc  func(me manifest.Entry) error
}

var _ dynamicaccess.Act = (*ActMock)(nil)

func (act *ActMock) Add(key []byte, val []byte) error {
	if act.AddFunc == nil {
		return nil
	}
	return act.AddFunc(key, val)
}

func (act *ActMock) Lookup(key []byte) ([]byte, error) {
	if act.LookupFunc == nil {
		return make([]byte, 0), nil
	}
	return act.LookupFunc(key)
}

func (act *ActMock) Load(key []byte) (manifest.Entry, error) {
	if act.LoadFunc == nil {
		return nil, nil
	}
	return act.LoadFunc(key)
}

func (act *ActMock) Store(me manifest.Entry) error {
	if act.StoreFunc == nil {
		return nil
	}
	act.StoreFunc(me)
	return nil
}

func NewActMock(addFunc func(key []byte, val []byte) error, getFunc func(key []byte) ([]byte, error)) dynamicaccess.Act {
	return &ActMock{
		AddFunc:    addFunc,
		LookupFunc: getFunc,
	}
}
