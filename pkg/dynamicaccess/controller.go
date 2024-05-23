// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type Grantees interface {
	// UpdateHandler manages the grantees for the given publisher, updating the list based on provided public keys to add or remove.
	// Only the publisher can make changes to the grantee list.
	UpdateHandler(ctx context.Context, ls file.LoadSaver, gls file.LoadSaver, granteeref swarm.Address, historyref swarm.Address, publisher *ecdsa.PublicKey, addList, removeList []*ecdsa.PublicKey) (swarm.Address, swarm.Address, swarm.Address, swarm.Address, error)
	// Get returns the list of grantees for the given publisher.
	// The list is accessible only by the publisher.
	Get(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey, encryptedglref swarm.Address) ([]*ecdsa.PublicKey, error)
}

type Controller interface {
	Grantees
	// DownloadHandler decrypts the encryptedRef using the lookupkey based on the history and timestamp.
	DownloadHandler(ctx context.Context, ls file.LoadSaver, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address, timestamp int64) (swarm.Address, error)
	// UploadHandler encrypts the reference and stores it in the history as the latest update.
	UploadHandler(ctx context.Context, ls file.LoadSaver, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error)
	io.Closer
}

type ControllerStruct struct {
	accessLogic ActLogic
}

var _ Controller = (*ControllerStruct)(nil)

func (c *ControllerStruct) DownloadHandler(
	ctx context.Context,
	ls file.LoadSaver,
	encryptedRef swarm.Address,
	publisher *ecdsa.PublicKey,
	historyRootHash swarm.Address,
	timestamp int64,
) (swarm.Address, error) {
	history, err := NewHistoryReference(ls, historyRootHash)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	entry, err := history.Lookup(ctx, timestamp)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	act, err := kvs.NewReference(ls, entry.Reference())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return c.accessLogic.DecryptRef(ctx, act, encryptedRef, publisher)
}

func (c *ControllerStruct) UploadHandler(
	ctx context.Context,
	ls file.LoadSaver,
	reference swarm.Address,
	publisher *ecdsa.PublicKey,
	historyRootHash swarm.Address,
) (swarm.Address, swarm.Address, swarm.Address, error) {
	historyRef := historyRootHash
	var (
		storage kvs.KeyValueStore
		actRef  swarm.Address
	)
	if historyRef.IsZero() {
		history, err := NewHistory(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		storage, err = kvs.New(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		err = c.accessLogic.AddGrantee(ctx, storage, publisher, publisher)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		actRef, err = storage.Save(ctx)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		err = history.Add(ctx, actRef, nil, nil)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		historyRef, err = history.Store(ctx)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	} else {
		history, err := NewHistoryReference(ls, historyRef)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		entry, err := history.Lookup(ctx, time.Now().Unix())
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		storage, err = kvs.NewReference(ls, entry.Reference())
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	encryptedRef, err := c.accessLogic.EncryptRef(ctx, storage, publisher, reference)
	return actRef, historyRef, encryptedRef, err
}

func NewController(accessLogic ActLogic) *ControllerStruct {
	return &ControllerStruct{
		accessLogic: accessLogic,
	}
}

func (c *ControllerStruct) UpdateHandler(
	ctx context.Context,
	ls file.LoadSaver,
	gls file.LoadSaver,
	encryptedglref swarm.Address,
	historyref swarm.Address,
	publisher *ecdsa.PublicKey,
	addList []*ecdsa.PublicKey,
	removeList []*ecdsa.PublicKey,
) (swarm.Address, swarm.Address, swarm.Address, swarm.Address, error) {
	var (
		err        error
		h          History
		act        kvs.KeyValueStore
		granteeref swarm.Address
	)
	if !historyref.IsZero() {
		h, err = NewHistoryReference(ls, historyref)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		entry, err := h.Lookup(ctx, time.Now().Unix())
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		actref := entry.Reference()
		act, err = kvs.NewReference(ls, actref)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	} else {
		h, err = NewHistory(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		// generate new access key and new act
		act, err = kvs.New(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		err = c.accessLogic.AddGrantee(ctx, act, publisher, publisher)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	var gl GranteeList
	if encryptedglref.IsZero() {
		gl, err = NewGranteeList(gls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	} else {
		granteeref, err = c.decryptRefForPublisher(publisher, encryptedglref)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}

		gl, err = NewGranteeListReference(ctx, gls, granteeref)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}
	if len(addList) != 0 {
		err = gl.Add(addList)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}
	granteesToAdd := addList
	if len(removeList) != 0 {
		err = gl.Remove(removeList)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		// generate new access key and new act
		act, err = kvs.New(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		err = c.accessLogic.AddGrantee(ctx, act, publisher, publisher)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		granteesToAdd = gl.Get()
	}

	for _, grantee := range granteesToAdd {
		err := c.accessLogic.AddGrantee(ctx, act, publisher, grantee)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	actref, err := act.Save(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	glref, err := gl.Save(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	eglref, err := c.encryptRefForPublisher(publisher, glref)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	// need to re-initialize history, because Lookup loads the forks causing the manifest save to skip the root node
	if !historyref.IsZero() {
		h, err = NewHistoryReference(ls, historyref)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	mtdt := map[string]string{"encryptedglref": eglref.String()}
	err = h.Add(ctx, actref, nil, &mtdt)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	href, err := h.Store(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	return glref, eglref, href, actref, nil
}

func (c *ControllerStruct) Get(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey, encryptedglref swarm.Address) ([]*ecdsa.PublicKey, error) {
	granteeRef, err := c.decryptRefForPublisher(publisher, encryptedglref)
	if err != nil {
		return nil, err
	}
	gl, err := NewGranteeListReference(ctx, ls, granteeRef)
	if err != nil {
		return nil, err
	}
	return gl.Get(), nil
}

func (c *ControllerStruct) encryptRefForPublisher(publisherPubKey *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error) {
	keys, err := c.accessLogic.Session.Key(publisherPubKey, [][]byte{oneByteArray})
	if err != nil {
		return swarm.ZeroAddress, err
	}
	refCipher := encryption.New(keys[0], 0, uint32(0), hashFunc)
	encryptedRef, err := refCipher.Encrypt(ref.Bytes())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(encryptedRef), nil
}

func (c *ControllerStruct) decryptRefForPublisher(publisherPubKey *ecdsa.PublicKey, encryptedRef swarm.Address) (swarm.Address, error) {
	keys, err := c.accessLogic.Session.Key(publisherPubKey, [][]byte{oneByteArray})
	if err != nil {
		return swarm.ZeroAddress, err
	}
	refCipher := encryption.New(keys[0], 0, uint32(0), hashFunc)
	ref, err := refCipher.Decrypt(encryptedRef.Bytes())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(ref), nil
}

// TODO: what to do in close ?
func (c *ControllerStruct) Close() error {
	return nil
}
