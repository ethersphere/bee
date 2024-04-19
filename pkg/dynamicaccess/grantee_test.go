package dynamicaccess_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
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
	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key3, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey, &key3.PublicKey}, nil
}

func TestGranteeAddGet(t *testing.T) {
	gl := dynamicaccess.NewGranteeList(createLs())
	keys, err := generateKeyListFixture()
	if err != nil {
		t.Errorf("key generation error: %v", err)
	}

	t.Run("Get empty grantee list should return error", func(t *testing.T) {
		val := gl.Get()
		assert.Nil(t, val)
	})

	t.Run("Get should return value equal to put value", func(t *testing.T) {
		var (
			addList1 []*ecdsa.PublicKey = []*ecdsa.PublicKey{keys[0]}
			addList2 []*ecdsa.PublicKey = []*ecdsa.PublicKey{keys[1], keys[0]}
			addList3 []*ecdsa.PublicKey = keys
		)
		testCases := []struct {
			name string
			list []*ecdsa.PublicKey
		}{
			{
				name: "Test list = 1",
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
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					expList = append(expList, tc.list...)
					retVal := gl.Get()
					assert.Equal(t, expList, retVal)
				}
			})
		}
	})
}

func TestGranteeRemove(t *testing.T) {
	gl := dynamicaccess.NewGranteeList(createLs())
	keys, err := generateKeyListFixture()
	if err != nil {
		t.Errorf("key generation error: %v", err)
	}

	t.Run("Add should NOT return error", func(t *testing.T) {
		err := gl.Add(keys)
		assert.NoError(t, err)
		retVal := gl.Get()
		assert.Equal(t, keys, retVal)
	})
	removeList1 := []*ecdsa.PublicKey{keys[0]}
	removeList2 := []*ecdsa.PublicKey{keys[2], keys[1]}
	t.Run("Remove the first item should return NO error", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assert.NoError(t, err)
		retVal := gl.Get()
		assert.Equal(t, removeList2, retVal)
	})
	t.Run("Remove non-existent item should return NO error", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assert.NoError(t, err)
		retVal := gl.Get()
		assert.Equal(t, removeList2, retVal)
	})
	t.Run("Remove second and third item should return NO error", func(t *testing.T) {
		err := gl.Remove(removeList2)
		assert.NoError(t, err)
		retVal := gl.Get()
		assert.Nil(t, retVal)
	})
	t.Run("Remove from empty grantee list should return error", func(t *testing.T) {
		err := gl.Remove(removeList1)
		assert.Error(t, err)
		retVal := gl.Get()
		assert.Nil(t, retVal)
	})
	t.Run("Remove empty remove list should return error", func(t *testing.T) {
		err := gl.Remove(nil)
		assert.Error(t, err)
		retVal := gl.Get()
		assert.Nil(t, retVal)
	})
}

func TestGranteeSave(t *testing.T) {
	ctx := context.Background()
	keys, err := generateKeyListFixture()
	if err != nil {
		t.Errorf("key generation error: %v", err)
	}
	t.Run("Save empty grantee list return NO error", func(t *testing.T) {
		gl := dynamicaccess.NewGranteeList(createLs())
		_, err := gl.Save(ctx)
		assert.NoError(t, err)
	})
	t.Run("Save not empty grantee list return valid swarm address", func(t *testing.T) {
		gl := dynamicaccess.NewGranteeList(createLs())
		err = gl.Add(keys)
		ref, err := gl.Save(ctx)
		assert.NoError(t, err)
		assert.True(t, ref.IsValidNonEmpty())
	})
	t.Run("Save grantee list with one item, no error, pre-save value exist", func(t *testing.T) {
		ls := createLs()
		gl1 := dynamicaccess.NewGranteeList(ls)

		err := gl1.Add(keys)
		assert.NoError(t, err)

		ref, err := gl1.Save(ctx)
		assert.NoError(t, err)

		gl2 := dynamicaccess.NewGranteeListReference(ls, ref)
		val := gl2.Get()
		assert.NoError(t, err)
		assert.Equal(t, keys, val)
	})
	t.Run("Save grantee list and add one item, no error, after-save value exist", func(t *testing.T) {
		ls := createLs()

		gl1 := dynamicaccess.NewGranteeList(ls)

		err := gl1.Add(keys)
		assert.NoError(t, err)
		ref, err := gl1.Save(ctx)
		assert.NoError(t, err)

		gl2 := dynamicaccess.NewGranteeListReference(ls, ref)
		err = gl2.Add(keys)
		assert.NoError(t, err)

		val := gl2.Get()
		assert.Equal(t, append(keys, keys...), val)
	})
}
