// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//nolint:ireturn
package kvs_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol/kvs"
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

//nolint:gochecknoglobals
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
	t.Helper()
	return swarm.RandAddress(t).Bytes(), swarm.RandAddress(t).Bytes()
}

func TestKvs(t *testing.T) {
	t.Parallel()
	s, err := kvs.New(createLs())
	assert.NoError(t, err)

	key, val := keyValuePair(t)
	ctx := context.Background()

	t.Run("Get non-existent key should return error", func(t *testing.T) {
		_, err := s.Get(ctx, []byte{1})
		assert.Error(t, err)
	})

	t.Run("Multiple Get with same key, no error", func(t *testing.T) {
		err := s.Put(ctx, key, val)
		assert.NoError(t, err)

		// get #1
		v, err := s.Get(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, val, v)
		// get #2
		v, err = s.Get(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, val, v)
	})

	t.Run("Get should return value equal to put value", func(t *testing.T) {
		var (
			key1 = []byte{1}
			key2 = []byte{2}
			key3 = []byte{3}
		)
		testCases := []struct {
			name string
			key  []byte
			val  []byte
		}{
			{
				name: "Test key = 1",
				key:  key1,
				val:  []byte{11},
			},
			{
				name: "Test key = 2",
				key:  key2,
				val:  []byte{22},
			},
			{
				name: "Test overwrite key = 1",
				key:  key1,
				val:  []byte{111},
			},
			{
				name: "Test key = 3",
				key:  key3,
				val:  []byte{33},
			},
			{
				name: "Test key = 3 with same value",
				key:  key3,
				val:  []byte{33},
			},
			{
				name: "Test key = 3 with value for key1",
				key:  key3,
				val:  []byte{11},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := s.Put(ctx, tc.key, tc.val)
				assert.NoError(t, err)
				retVal, err := s.Get(ctx, tc.key)
				assert.NoError(t, err)
				assert.Equal(t, tc.val, retVal)
			})
		}
	})
}

func TestKvs_Save(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	key1, val1 := keyValuePair(t)
	key2, val2 := keyValuePair(t)
	t.Run("Save empty KVS return error", func(t *testing.T) {
		s, _ := kvs.New(createLs())
		_, err := s.Save(ctx)
		assert.Error(t, err)
	})
	t.Run("Save not empty KVS return valid swarm address", func(t *testing.T) {
		s, _ := kvs.New(createLs())
		_ = s.Put(ctx, key1, val1)
		ref, err := s.Save(ctx)
		assert.NoError(t, err)
		assert.True(t, ref.IsValidNonEmpty())
	})
	t.Run("Save KVS with one item, no error, pre-save value exist", func(t *testing.T) {
		ls := createLs()
		s1, _ := kvs.New(ls)

		err := s1.Put(ctx, key1, val1)
		assert.NoError(t, err)

		ref, err := s1.Save(ctx)
		assert.NoError(t, err)

		s2, err := kvs.NewReference(ls, ref)
		assert.NoError(t, err)

		val, err := s2.Get(ctx, key1)
		assert.NoError(t, err)
		assert.Equal(t, val1, val)
	})
	t.Run("Save KVS and add one item, no error, after-save value exist", func(t *testing.T) {
		ls := createLs()

		kvs1, _ := kvs.New(ls)

		err := kvs1.Put(ctx, key1, val1)
		assert.NoError(t, err)
		ref, err := kvs1.Save(ctx)
		assert.NoError(t, err)

		// New KVS
		kvs2, err := kvs.NewReference(ls, ref)
		assert.NoError(t, err)
		err = kvs2.Put(ctx, key2, val2)
		assert.NoError(t, err)

		val, err := kvs2.Get(ctx, key2)
		assert.NoError(t, err)
		assert.Equal(t, val2, val)
	})
}
