// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"encoding/binary"
	"errors"
	"strconv"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

// step_01 serves as example for setting up migration step.
//
// In this example migration step will ensure that dummyObj with id:1635 is
// defined in the store.
func step_01(s storage.Store) error {
	obj := newDummyObj(1635)

	if has, err := s.Has(obj); err != nil {
		return err
	} else if !has {
		return s.Put(obj)
	}

	return nil
}

type dummyObj struct {
	id int
}

func newDummyObj(id int) *dummyObj {
	return &dummyObj{id: id}
}

func (o *dummyObj) ID() string     { return strconv.Itoa(o.id) }
func (dummyObj) Namespace() string { return "dummy-obj" }

func (o *dummyObj) Marshal() ([]byte, error) {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf, uint64(o.id))
	return buf, nil
}

func (o *dummyObj) Unmarshal(buf []byte) error {
	if len(buf) != 16 {
		return errors.New("invalid length")
	}
	o.id = int(binary.LittleEndian.Uint64(buf))
	return nil
}

func (o *dummyObj) Clone() storage.Item {
	if o == nil {
		return nil
	}
	return &dummyObj{
		id: o.id,
	}
}
