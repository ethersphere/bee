// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
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

func generateKeyListFixture() ([]*ecdsa.PublicKey, error) {
	key1, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	key2, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	key3, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey, &key3.PublicKey}, nil
}

func TestGranteeAddGet(t *testing.T) {
	t.Parallel()
	gl := accesscontrol.NewGranteeList(createLs())
	keys, err := generateKeyListFixture()
	assertNoError(t, "key generation", err)

	t.Run("Get empty grantee list should return", func(t *testing.T) {
		val := gl.Get()
		assert.Empty(t, val)
	})

	t.Run("Get should return value equal to put value", func(t *testing.T) {
		var (
			keys2, err = generateKeyListFixture()
			addList1   = []*ecdsa.PublicKey{keys[0]}
			addList2   = []*ecdsa.PublicKey{keys[1], keys[2]}
			addList3   = keys2
		)
		assertNoError(t, "key generation", err)
		testCases := []struct {
			name string
			list []*ecdsa.PublicKey
		}{
			{
				name: "Test list = 1",
				list: addList1,
			},
			{
				name: "Test list = duplicate1",
				list: addList1,
			},
			{
				name: "Test list = 2",
				list: addList2,
			},
			{
				name: "Test list = 3",
				list: addList3,
			},
			{
				name: "Test empty add list",
				list: nil,
			},
		}

		expList := []*ecdsa.PublicKey{}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := gl.Add(tc.list)
				if tc.list == nil {
					assertError(t, "granteelist add", err)
				} else {
					assertNoError(t, "granteelist add", err)
					if tc.name != "Test list = duplicate1" {
						expList = append(expList, tc.list...)
					}
					retVal := gl.Get()
					assert.Equal(t, expList, retVal)
				}
			})
		}
	})
}

func TestGranteeRemove(t *testing.T) {
	t.Parallel()
	gl := accesscontrol.NewGranteeList(createLs())
	keys, err := generateKeyListFixture()
	assertNoError(t, "key generation", err)

	t.Run("Add should NOT return", func(t *testing.T) {
		err := gl.Add(keys)
		assertNoError(t, "granteelist add", err)
		retVal := gl.Get()
		assert.Equal(t, keys, retVal)
	})
	removeList1 := []*ecdsa.PublicKey{keys[0]}
	removeList2 := []*ecdsa.PublicKey{keys[2], keys[1]}
	t.Run("Remove the first item should return NO", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assertNoError(t, "granteelist remove", err)
		retVal := gl.Get()
		assert.Equal(t, removeList2, retVal)
	})
	t.Run("Remove non-existent item should return NO", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assertNoError(t, "granteelist remove", err)
		retVal := gl.Get()
		assert.Equal(t, removeList2, retVal)
	})
	t.Run("Remove second and third item should return NO", func(t *testing.T) {
		err := gl.Remove(removeList2)
		assertNoError(t, "granteelist remove", err)
		retVal := gl.Get()
		assert.Empty(t, retVal)
	})
	t.Run("Remove from empty grantee list should return", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assertError(t, "remove from empty grantee list", err)
		retVal := gl.Get()
		assert.Empty(t, retVal)
	})
	t.Run("Remove empty remove list should return", func(t *testing.T) {
		err := gl.Remove(nil)
		assertError(t, "remove empty list", err)
		retVal := gl.Get()
		assert.Empty(t, retVal)
	})
}

func TestGranteeSave(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	keys, err := generateKeyListFixture()
	assertNoError(t, "key generation", err)

	t.Run("Create grantee list with invalid reference, expect", func(t *testing.T) {
		gl, err := accesscontrol.NewGranteeListReference(ctx, createLs(), swarm.RandAddress(t))
		assertError(t, "create grantee list ref", err)
		assert.Nil(t, gl)
	})
	t.Run("Save empty grantee list return NO", func(t *testing.T) {
		gl := accesscontrol.NewGranteeList(createLs())
		_, err := gl.Save(ctx)
		assertNoError(t, "granteelist save", err)
	})
	t.Run("Save not empty grantee list return valid swarm address", func(t *testing.T) {
		gl := accesscontrol.NewGranteeList(createLs())
		err = gl.Add(keys)
		assertNoError(t, "granteelist add", err)
		ref, err := gl.Save(ctx)
		assertNoError(t, "granteelist save", err)
		assert.True(t, ref.IsValidNonEmpty())
	})
	t.Run("Save grantee list with one item, no error, pre-save value exist", func(t *testing.T) {
		ls := createLs()
		gl1 := accesscontrol.NewGranteeList(ls)

		err := gl1.Add(keys)
		assertNoError(t, "granteelist add", err)

		ref, err := gl1.Save(ctx)
		assertNoError(t, "1st granteelist save", err)

		gl2, err := accesscontrol.NewGranteeListReference(ctx, ls, ref)
		assertNoError(t, "create grantee list ref", err)
		val := gl2.Get()
		assertNoError(t, "2nd granteelist save", err)
		assert.Equal(t, keys, val)
	})
	t.Run("Save grantee list and add one item, no error, after-save value exist", func(t *testing.T) {
		ls := createLs()
		keys2, err := generateKeyListFixture()
		assertNoError(t, "key generation", err)

		gl1 := accesscontrol.NewGranteeList(ls)

		err = gl1.Add(keys)
		assertNoError(t, "granteelist1 add", err)

		ref, err := gl1.Save(ctx)
		assertNoError(t, "granteelist1 save", err)

		gl2, err := accesscontrol.NewGranteeListReference(ctx, ls, ref)
		assertNoError(t, "create grantee list ref", err)
		err = gl2.Add(keys2)
		assertNoError(t, "create grantee list ref", err)

		val := gl2.Get()
		assert.Equal(t, append(keys, keys2...), val)
	})
}

func TestGranteeRemoveTwo(t *testing.T) {
	gl := accesscontrol.NewGranteeList(createLs())
	keys, err := generateKeyListFixture()
	assertNoError(t, "key generation", err)
	err = gl.Add([]*ecdsa.PublicKey{keys[0]})
	assertNoError(t, "1st granteelist add", err)
	err = gl.Add([]*ecdsa.PublicKey{keys[0]})
	assertNoError(t, "2nd granteelist add", err)
	err = gl.Remove([]*ecdsa.PublicKey{keys[0]})
	assertNoError(t, "granteelist remove", err)
}
