// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess_test

import (
	"context"
	"crypto/ecdsa"
	"reflect"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
	encryption "github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

//nolint:errcheck,gosec,wrapcheck
func getHistoryFixture(ctx context.Context, ls file.LoadSaver, al dynamicaccess.ActLogic, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	h, err := dynamicaccess.NewHistory(ls)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	pk1 := getPrivKey(1)
	pk2 := getPrivKey(2)

	kvs0, _ := kvs.New(ls)
	al.AddGrantee(ctx, kvs0, publisher, publisher)
	kvs0Ref, _ := kvs0.Save(ctx)
	kvs1, _ := kvs.New(ls)
	al.AddGrantee(ctx, kvs1, publisher, publisher)
	al.AddGrantee(ctx, kvs1, publisher, &pk1.PublicKey)
	kvs1Ref, _ := kvs1.Save(ctx)
	kvs2, _ := kvs.New(ls)
	al.AddGrantee(ctx, kvs2, publisher, publisher)
	al.AddGrantee(ctx, kvs2, publisher, &pk2.PublicKey)
	kvs2Ref, _ := kvs2.Save(ctx)
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	secondTime := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	thirdTime := time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()

	h.Add(ctx, kvs0Ref, &thirdTime, nil)
	h.Add(ctx, kvs1Ref, &firstTime, nil)
	h.Add(ctx, kvs2Ref, &secondTime, nil)
	return h.Store(ctx)
}

func TestController_UploadHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(0)
	diffieHellman := dynamicaccess.NewDefaultSession(publisher)
	al := dynamicaccess.NewLogic(diffieHellman)
	c := dynamicaccess.NewController(al)
	ls := createLs()

	t.Run("New upload", func(t *testing.T) {
		ref := swarm.RandAddress(t)
		_, hRef, encRef, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, swarm.ZeroAddress)
		assert.NoError(t, err)

		h, _ := dynamicaccess.NewHistoryReference(ls, hRef)
		entry, _ := h.Lookup(ctx, time.Now().Unix())
		actRef := entry.Reference()
		act, _ := kvs.NewReference(ls, actRef)
		expRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)

		assert.NoError(t, err)
		assert.Equal(t, encRef, expRef)
		assert.NotEqual(t, hRef, swarm.ZeroAddress)
	})

	t.Run("Upload to same history", func(t *testing.T) {
		ref := swarm.RandAddress(t)
		_, hRef1, _, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, swarm.ZeroAddress)
		assert.NoError(t, err)
		_, hRef2, encRef, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, hRef1)
		assert.NoError(t, err)
		h, err := dynamicaccess.NewHistoryReference(ls, hRef2)
		assert.NoError(t, err)
		hRef2, err = h.Store(ctx)
		assert.NoError(t, err)
		assert.True(t, hRef1.Equal(hRef2))

		h, _ = dynamicaccess.NewHistoryReference(ls, hRef2)
		entry, _ := h.Lookup(ctx, time.Now().Unix())
		actRef := entry.Reference()
		act, _ := kvs.NewReference(ls, actRef)
		expRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)

		assert.NoError(t, err)
		assert.Equal(t, encRef, expRef)
		assert.NotEqual(t, hRef2, swarm.ZeroAddress)
	})
}

func TestController_PublisherDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(0)
	diffieHellman := dynamicaccess.NewDefaultSession(publisher)
	al := dynamicaccess.NewLogic(diffieHellman)
	c := dynamicaccess.NewController(al)
	ls := createLs()
	ref := swarm.RandAddress(t)
	href, _ := getHistoryFixture(ctx, ls, al, &publisher.PublicKey)
	h, _ := dynamicaccess.NewHistoryReference(ls, href)
	entry, _ := h.Lookup(ctx, time.Now().Unix())
	actRef := entry.Reference()
	act, _ := kvs.NewReference(ls, actRef)
	encRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)

	assert.NoError(t, err)
	dref, err := c.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, href, time.Now().Unix())
	assert.NoError(t, err)
	assert.Equal(t, ref, dref)
}

func TestController_GranteeDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(0)
	grantee := getPrivKey(2)
	publisherDH := dynamicaccess.NewDefaultSession(publisher)
	publisherAL := dynamicaccess.NewLogic(publisherDH)

	diffieHellman := dynamicaccess.NewDefaultSession(grantee)
	al := dynamicaccess.NewLogic(diffieHellman)
	ls := createLs()
	c := dynamicaccess.NewController(al)
	ref := swarm.RandAddress(t)
	href, _ := getHistoryFixture(ctx, ls, publisherAL, &publisher.PublicKey)
	h, _ := dynamicaccess.NewHistoryReference(ls, href)
	ts := time.Date(2001, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, _ := h.Lookup(ctx, ts)
	actRef := entry.Reference()
	act, _ := kvs.NewReference(ls, actRef)
	encRef, err := publisherAL.EncryptRef(ctx, act, &publisher.PublicKey, ref)

	assert.NoError(t, err)
	dref, err := c.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, href, ts)
	assert.NoError(t, err)
	assert.Equal(t, ref, dref)
}

func TestController_UpdateHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(1)
	diffieHellman := dynamicaccess.NewDefaultSession(publisher)
	al := dynamicaccess.NewLogic(diffieHellman)
	keys, _ := al.Session.Key(&publisher.PublicKey, [][]byte{{1}})
	refCipher := encryption.New(keys[0], 0, uint32(0), sha3.NewLegacyKeccak256)
	ls := createLs()
	gls := loadsave.New(mockStorer.ChunkStore(), mockStorer.Cache(), requestPipelineFactory(context.Background(), mockStorer.Cache(), true, redundancy.NONE))
	c := dynamicaccess.NewController(al)
	href, _ := getHistoryFixture(ctx, ls, al, &publisher.PublicKey)

	grantee1 := getPrivKey(0)
	grantee := getPrivKey(2)

	t.Run("add to new list", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, _, _, _, err := c.UpdateHandler(ctx, ls, ls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)
		assert.NoError(t, err)

		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)

		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 1)
	})
	t.Run("add to existing list", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, eglref, _, _, err := c.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, href, &publisher.PublicKey, addList, nil)
		assert.NoError(t, err)

		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)

		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 1)

		addList = []*ecdsa.PublicKey{&getPrivKey(0).PublicKey}
		granteeRef, _, _, _, _ = c.UpdateHandler(ctx, ls, ls, eglref, href, &publisher.PublicKey, addList, nil)
		gl, err = dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)
		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 2)
	})
	t.Run("add and revoke", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		revokeList := []*ecdsa.PublicKey{&grantee1.PublicKey}
		gl, _ := dynamicaccess.NewGranteeList(ls)
		_ = gl.Add([]*ecdsa.PublicKey{&publisher.PublicKey, &grantee1.PublicKey})
		granteeRef, _ := gl.Save(ctx)
		eglref, _ := refCipher.Encrypt(granteeRef.Bytes())

		granteeRef, _, _, _, _ = c.UpdateHandler(ctx, ls, gls, swarm.NewAddress(eglref), href, &publisher.PublicKey, addList, revokeList)
		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)

		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 2)
	})
	t.Run("add and revoke then get from history", func(t *testing.T) {
		addRevokeList := []*ecdsa.PublicKey{&grantee.PublicKey}
		ref := swarm.RandAddress(t)
		_, hRef, encRef, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, swarm.ZeroAddress)
		require.NoError(t, err)

		// Need to wait a second before each update call so that a new history mantaray fork is created for the new key(timestamp) entry
		time.Sleep(1 * time.Second)
		beforeRevokeTS := time.Now().Unix()
		_, egranteeRef, hrefUpdate1, _, err := c.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, hRef, &publisher.PublicKey, addRevokeList, nil)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)
		granteeRef, _, hrefUpdate2, _, err := c.UpdateHandler(ctx, ls, gls, egranteeRef, hrefUpdate1, &publisher.PublicKey, nil, addRevokeList)
		require.NoError(t, err)

		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)
		require.NoError(t, err)
		assert.Empty(t, gl.Get())
		// expect history reference to be different after grantee list update
		assert.NotEqual(t, hrefUpdate1, hrefUpdate2)

		granteeDH := dynamicaccess.NewDefaultSession(grantee)
		granteeAl := dynamicaccess.NewLogic(granteeDH)
		granteeCtrl := dynamicaccess.NewController(granteeAl)
		// download with grantee shall still work with the timestamp before the revoke
		decRef, err := granteeCtrl.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, hrefUpdate2, beforeRevokeTS)
		require.NoError(t, err)
		assert.Equal(t, ref, decRef)

		// download with grantee shall NOT work with the latest timestamp
		decRef, err = granteeCtrl.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, hrefUpdate2, time.Now().Unix())
		require.Error(t, err)
		assert.Equal(t, swarm.ZeroAddress, decRef)

		// publisher shall still be able to download with the timestamp before the revoke
		decRef, err = c.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, hrefUpdate2, beforeRevokeTS)
		require.NoError(t, err)
		assert.Equal(t, ref, decRef)
	})
	t.Run("add twice", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey, &grantee.PublicKey}
		//nolint:ineffassign,staticcheck,wastedassign
		granteeRef, eglref, _, _, err := c.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, href, &publisher.PublicKey, addList, nil)
		granteeRef, _, _, _, _ = c.UpdateHandler(ctx, ls, ls, eglref, href, &publisher.PublicKey, addList, nil)
		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)

		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 1)
	})
	t.Run("revoke non-existing", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, _, _, _, _ := c.UpdateHandler(ctx, ls, ls, swarm.ZeroAddress, href, &publisher.PublicKey, addList, nil)
		gl, err := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)

		assert.NoError(t, err)
		assert.Len(t, gl.Get(), 1)
	})
}

func TestController_Get(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(1)
	caller := getPrivKey(0)
	grantee := getPrivKey(2)
	diffieHellman1 := dynamicaccess.NewDefaultSession(publisher)
	diffieHellman2 := dynamicaccess.NewDefaultSession(caller)
	al1 := dynamicaccess.NewLogic(diffieHellman1)
	al2 := dynamicaccess.NewLogic(diffieHellman2)
	ls := createLs()
	gls := loadsave.New(mockStorer.ChunkStore(), mockStorer.Cache(), requestPipelineFactory(context.Background(), mockStorer.Cache(), true, redundancy.NONE))
	c1 := dynamicaccess.NewController(al1)
	c2 := dynamicaccess.NewController(al2)

	t.Run("get by publisher", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, eglRef, _, _, _ := c1.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)

		grantees, err := c1.Get(ctx, ls, &publisher.PublicKey, eglRef)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(grantees, addList))

		gl, _ := dynamicaccess.NewGranteeListReference(ctx, ls, granteeRef)
		assert.True(t, reflect.DeepEqual(gl.Get(), addList))
	})
	t.Run("get by non-publisher", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		_, eglRef, _, _, _ := c1.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)
		grantees, err := c2.Get(ctx, ls, &publisher.PublicKey, eglRef)
		assert.Error(t, err)
		assert.Nil(t, grantees)
	})
}
