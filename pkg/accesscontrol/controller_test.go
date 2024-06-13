// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"crypto/ecdsa"
	"reflect"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol/kvs"
	encryption "github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

//nolint:errcheck,gosec,wrapcheck
func getHistoryFixture(t *testing.T, ctx context.Context, ls file.LoadSaver, al accesscontrol.ActLogic, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	t.Helper()
	h, err := accesscontrol.NewHistory(ls)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	pk1 := getPrivKey(1)
	pk2 := getPrivKey(2)

	kvs0, err := kvs.New(ls)
	assertNoError(t, "kvs0 create", err)
	al.AddGrantee(ctx, kvs0, publisher, publisher)
	kvs0Ref, err := kvs0.Save(ctx)
	assertNoError(t, "kvs0 save", err)
	kvs1, err := kvs.New(ls)
	assertNoError(t, "kvs1 create", err)
	al.AddGrantee(ctx, kvs1, publisher, publisher)
	al.AddGrantee(ctx, kvs1, publisher, &pk1.PublicKey)
	kvs1Ref, err := kvs1.Save(ctx)
	assertNoError(t, "kvs1 save", err)
	kvs2, err := kvs.New(ls)
	assertNoError(t, "kvs2 create", err)
	al.AddGrantee(ctx, kvs2, publisher, publisher)
	al.AddGrantee(ctx, kvs2, publisher, &pk2.PublicKey)
	kvs2Ref, err := kvs2.Save(ctx)
	assertNoError(t, "kvs2 save", err)
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
	diffieHellman := accesscontrol.NewDefaultSession(publisher)
	al := accesscontrol.NewLogic(diffieHellman)
	c := accesscontrol.NewController(al)
	ls := createLs()

	t.Run("New upload", func(t *testing.T) {
		ref := swarm.RandAddress(t)
		_, hRef, encRef, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, swarm.ZeroAddress)
		assertNoError(t, "UploadHandler", err)

		h, err := accesscontrol.NewHistoryReference(ls, hRef)
		assertNoError(t, "create history ref", err)
		entry, err := h.Lookup(ctx, time.Now().Unix())
		assertNoError(t, "history lookup", err)
		actRef := entry.Reference()
		act, err := kvs.NewReference(ls, actRef)
		assertNoError(t, "kvs create ref", err)
		expRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)

		assertNoError(t, "encrypt ref", err)
		assert.Equal(t, encRef, expRef)
		assert.NotEqual(t, hRef, swarm.ZeroAddress)
	})

	t.Run("Upload to same history", func(t *testing.T) {
		ref := swarm.RandAddress(t)
		_, hRef1, _, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, swarm.ZeroAddress)
		assertNoError(t, "1st upload", err)
		_, hRef2, encRef, err := c.UploadHandler(ctx, ls, ref, &publisher.PublicKey, hRef1)
		assertNoError(t, "2nd upload", err)
		h, err := accesscontrol.NewHistoryReference(ls, hRef2)
		assertNoError(t, "create history ref", err)
		hRef2, err = h.Store(ctx)
		assertNoError(t, "store history", err)
		assert.True(t, hRef1.Equal(hRef2))

		h, err = accesscontrol.NewHistoryReference(ls, hRef2)
		assertNoError(t, "create history ref", err)
		entry, err := h.Lookup(ctx, time.Now().Unix())
		assertNoError(t, "history lookup", err)
		actRef := entry.Reference()
		act, err := kvs.NewReference(ls, actRef)
		assertNoError(t, "kvs create ref", err)
		expRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)

		assertNoError(t, "encrypt ref", err)
		assert.Equal(t, expRef, encRef)
		assert.NotEqual(t, hRef2, swarm.ZeroAddress)
	})
}

func TestController_PublisherDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(0)
	diffieHellman := accesscontrol.NewDefaultSession(publisher)
	al := accesscontrol.NewLogic(diffieHellman)
	c := accesscontrol.NewController(al)
	ls := createLs()
	ref := swarm.RandAddress(t)
	href, err := getHistoryFixture(t, ctx, ls, al, &publisher.PublicKey)
	assertNoError(t, "history fixture create", err)
	h, err := accesscontrol.NewHistoryReference(ls, href)
	assertNoError(t, "create history ref", err)
	entry, err := h.Lookup(ctx, time.Now().Unix())
	assertNoError(t, "history lookup", err)
	actRef := entry.Reference()
	act, err := kvs.NewReference(ls, actRef)
	assertNoError(t, "kvs create ref", err)
	encRef, err := al.EncryptRef(ctx, act, &publisher.PublicKey, ref)
	assertNoError(t, "encrypt ref", err)

	dref, err := c.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, href, time.Now().Unix())
	assertNoError(t, "download by publisher", err)
	assert.Equal(t, ref, dref)
}

func TestController_GranteeDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(0)
	grantee := getPrivKey(2)
	publisherDH := accesscontrol.NewDefaultSession(publisher)
	publisherAL := accesscontrol.NewLogic(publisherDH)

	diffieHellman := accesscontrol.NewDefaultSession(grantee)
	al := accesscontrol.NewLogic(diffieHellman)
	ls := createLs()
	c := accesscontrol.NewController(al)
	ref := swarm.RandAddress(t)
	href, err := getHistoryFixture(t, ctx, ls, publisherAL, &publisher.PublicKey)
	assertNoError(t, "history fixture create", err)
	h, err := accesscontrol.NewHistoryReference(ls, href)
	assertNoError(t, "history fixture create", err)
	ts := time.Date(2001, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	entry, err := h.Lookup(ctx, ts)
	assertNoError(t, "history lookup", err)
	actRef := entry.Reference()
	act, err := kvs.NewReference(ls, actRef)
	assertNoError(t, "kvs create ref", err)
	encRef, err := publisherAL.EncryptRef(ctx, act, &publisher.PublicKey, ref)

	assertNoError(t, "encrypt ref", err)
	dref, err := c.DownloadHandler(ctx, ls, encRef, &publisher.PublicKey, href, ts)
	assertNoError(t, "download by grantee", err)
	assert.Equal(t, ref, dref)
}

func TestController_UpdateHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(1)
	diffieHellman := accesscontrol.NewDefaultSession(publisher)
	al := accesscontrol.NewLogic(diffieHellman)
	keys, err := al.Session.Key(&publisher.PublicKey, [][]byte{{1}})
	assertNoError(t, "Session key", err)
	refCipher := encryption.New(keys[0], 0, 0, sha3.NewLegacyKeccak256)
	ls := createLs()
	gls := loadsave.New(mockStorer.ChunkStore(), mockStorer.Cache(), requestPipelineFactory(context.Background(), mockStorer.Cache(), true, redundancy.NONE))
	c := accesscontrol.NewController(al)
	href, err := getHistoryFixture(t, ctx, ls, al, &publisher.PublicKey)
	assertNoError(t, "history fixture create", err)

	grantee1 := getPrivKey(0)
	grantee := getPrivKey(2)

	t.Run("add to new list", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, _, _, _, err := c.UpdateHandler(ctx, ls, ls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandlererror", err)

		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)

		assertNoError(t, "create granteelist ref", err)
		assert.Len(t, gl.Get(), 1)
	})
	t.Run("add to existing list", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, eglref, _, _, err := c.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, href, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandlererror", err)

		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)

		assertNoError(t, "create granteelist ref", err)
		assert.Len(t, gl.Get(), 1)

		addList = []*ecdsa.PublicKey{&getPrivKey(0).PublicKey}
		granteeRef, _, _, _, err = c.UpdateHandler(ctx, ls, ls, eglref, href, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandler", err)
		gl, err = accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)
		assertNoError(t, "create granteelist ref", err)
		assert.Len(t, gl.Get(), 2)
	})
	t.Run("add and revoke", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		revokeList := []*ecdsa.PublicKey{&grantee1.PublicKey}
		gl := accesscontrol.NewGranteeList(ls)
		err = gl.Add([]*ecdsa.PublicKey{&publisher.PublicKey, &grantee1.PublicKey})
		granteeRef, err := gl.Save(ctx)
		assertNoError(t, "granteelist save", err)
		eglref, err := refCipher.Encrypt(granteeRef.Bytes())
		assertNoError(t, "encrypt granteeref", err)

		granteeRef, _, _, _, err = c.UpdateHandler(ctx, ls, gls, swarm.NewAddress(eglref), href, &publisher.PublicKey, addList, revokeList)
		assertNoError(t, "UpdateHandler", err)
		gl, err = accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)

		assertNoError(t, "create granteelist ref", err)
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

		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)
		require.NoError(t, err)
		assert.Empty(t, gl.Get())
		// expect history reference to be different after grantee list update
		assert.NotEqual(t, hrefUpdate1, hrefUpdate2)

		granteeDH := accesscontrol.NewDefaultSession(grantee)
		granteeAl := accesscontrol.NewLogic(granteeDH)
		granteeCtrl := accesscontrol.NewController(granteeAl)
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
		granteeRef, _, _, _, err = c.UpdateHandler(ctx, ls, ls, eglref, href, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandler", err)
		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)

		assertNoError(t, "create granteelist ref", err)
		assert.Len(t, gl.Get(), 1)
	})
	t.Run("revoke non-existing", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, _, _, _, err := c.UpdateHandler(ctx, ls, ls, swarm.ZeroAddress, href, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandler", err)
		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)

		assertNoError(t, "create granteelist ref", err)
		assert.Len(t, gl.Get(), 1)
	})
}

func TestController_Get(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	publisher := getPrivKey(1)
	caller := getPrivKey(0)
	grantee := getPrivKey(2)
	diffieHellman1 := accesscontrol.NewDefaultSession(publisher)
	diffieHellman2 := accesscontrol.NewDefaultSession(caller)
	al1 := accesscontrol.NewLogic(diffieHellman1)
	al2 := accesscontrol.NewLogic(diffieHellman2)
	ls := createLs()
	gls := loadsave.New(mockStorer.ChunkStore(), mockStorer.Cache(), requestPipelineFactory(context.Background(), mockStorer.Cache(), true, redundancy.NONE))
	c1 := accesscontrol.NewController(al1)
	c2 := accesscontrol.NewController(al2)

	t.Run("get by publisher", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		granteeRef, eglRef, _, _, err := c1.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandler", err)

		grantees, err := c1.Get(ctx, ls, &publisher.PublicKey, eglRef)
		assertNoError(t, "get by publisher", err)
		assert.True(t, reflect.DeepEqual(grantees, addList))

		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, granteeRef)
		assertNoError(t, "create granteelist ref", err)
		assert.True(t, reflect.DeepEqual(gl.Get(), addList))
	})
	t.Run("get by non-publisher", func(t *testing.T) {
		addList := []*ecdsa.PublicKey{&grantee.PublicKey}
		_, eglRef, _, _, err := c1.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, swarm.ZeroAddress, &publisher.PublicKey, addList, nil)
		assertNoError(t, "UpdateHandler", err)
		grantees, err := c2.Get(ctx, ls, &publisher.PublicKey, eglRef)
		assertError(t, "controller get by non-publisher", err)
		assert.Nil(t, grantees)
	})
}
