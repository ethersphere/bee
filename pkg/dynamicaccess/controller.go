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
	UpdateHandler(ctx context.Context, ls file.LoadSaver, gls file.LoadSaver, granteeRef swarm.Address, historyRef swarm.Address, publisher *ecdsa.PublicKey, addList, removeList []*ecdsa.PublicKey) (swarm.Address, swarm.Address, swarm.Address, swarm.Address, error)
	// Get returns the list of grantees for the given publisher.
	// The list is accessible only by the publisher.
	Get(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey, encryptedglRef swarm.Address) ([]*ecdsa.PublicKey, error)
}

type Controller interface {
	Grantees
	// DownloadHandler decrypts the encryptedRef using the lookupkey based on the history and timestamp.
	DownloadHandler(ctx context.Context, ls file.LoadSaver, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRef swarm.Address, timestamp int64) (swarm.Address, error)
	// UploadHandler encrypts the reference and stores it in the history as the latest update.
	UploadHandler(ctx context.Context, ls file.LoadSaver, reference swarm.Address, publisher *ecdsa.PublicKey, historyRef swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error)
	io.Closer
}

type ControllerStruct struct {
	accessLogic ActLogic
}

var _ Controller = (*ControllerStruct)(nil)

func NewController(accessLogic ActLogic) *ControllerStruct {
	return &ControllerStruct{
		accessLogic: accessLogic,
	}
}

func (c *ControllerStruct) DownloadHandler(
	ctx context.Context,
	ls file.LoadSaver,
	encryptedRef swarm.Address,
	publisher *ecdsa.PublicKey,
	historyRef swarm.Address,
	timestamp int64,
) (swarm.Address, error) {
	_, act, err := c.getHistoryAndAct(ctx, ls, historyRef, publisher, timestamp)
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
	historyRef swarm.Address,
) (swarm.Address, swarm.Address, swarm.Address, error) {
	history, act, err := c.getHistoryAndAct(ctx, ls, historyRef, publisher, time.Now().Unix())
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	actRef := swarm.ZeroAddress
	newHistoryRef := historyRef
	if historyRef.IsZero() {
		newHistoryRef, actRef, err = c.saveHistoryAndAct(ctx, history, nil, act)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	encryptedRef, err := c.accessLogic.EncryptRef(ctx, act, publisher, reference)
	return actRef, newHistoryRef, encryptedRef, err
}

// Limitation: If an upadate is called again within a second from the latest upload/update then mantaray save fails with ErrInvalidInput,
// because the key (timestamp) is already present, hence a new fork is not created
func (c *ControllerStruct) UpdateHandler(
	ctx context.Context,
	ls file.LoadSaver,
	gls file.LoadSaver,
	encryptedglRef swarm.Address,
	historyRef swarm.Address,
	publisher *ecdsa.PublicKey,
	addList []*ecdsa.PublicKey,
	removeList []*ecdsa.PublicKey,
) (swarm.Address, swarm.Address, swarm.Address, swarm.Address, error) {
	history, act, err := c.getHistoryAndAct(ctx, ls, historyRef, publisher, time.Now().Unix())
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	gl, err := c.getGranteeList(ctx, gls, encryptedglRef, publisher)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
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
		// generate new access key and new act, only if history was not newly created
		if !historyRef.IsZero() {
			act, err = c.newActWithPublisher(ctx, ls, publisher)
			if err != nil {
				return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
			}
		}
		granteesToAdd = gl.Get()
	}

	for _, grantee := range granteesToAdd {
		err := c.accessLogic.AddGrantee(ctx, act, publisher, grantee)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	granteeRef, err := gl.Save(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	egranteeRef, err := c.encryptRefForPublisher(publisher, granteeRef)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	// need to re-initialize history, because Lookup loads the forks causing the manifest save to skip the root node
	if !historyRef.IsZero() {
		history, err = NewHistoryReference(ls, historyRef)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	mtdt := map[string]string{"encryptedglref": egranteeRef.String()}
	hRef, actRef, err := c.saveHistoryAndAct(ctx, history, &mtdt, act)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	return granteeRef, egranteeRef, hRef, actRef, nil
}

func (c *ControllerStruct) Get(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey, encryptedglRef swarm.Address) ([]*ecdsa.PublicKey, error) {
	gl, err := c.getGranteeList(ctx, ls, encryptedglRef, publisher)
	if err != nil {
		return nil, err
	}
	return gl.Get(), nil
}

func (c *ControllerStruct) newActWithPublisher(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey) (kvs.KeyValueStore, error) {
	act, err := kvs.New(ls)
	if err != nil {
		return nil, err
	}
	err = c.accessLogic.AddGrantee(ctx, act, publisher, publisher)
	if err != nil {
		return nil, err
	}

	return act, nil
}

func (c *ControllerStruct) getHistoryAndAct(ctx context.Context, ls file.LoadSaver, historyRef swarm.Address, publisher *ecdsa.PublicKey, timestamp int64) (history History, act kvs.KeyValueStore, err error) {
	if historyRef.IsZero() {
		history, err = NewHistory(ls)
		if err != nil {
			return nil, nil, err
		}
		act, err = c.newActWithPublisher(ctx, ls, publisher)
		if err != nil {
			return nil, nil, err
		}
	} else {
		history, err = NewHistoryReference(ls, historyRef)
		if err != nil {
			return nil, nil, err
		}
		entry, err := history.Lookup(ctx, timestamp)
		if err != nil {
			return nil, nil, err
		}
		act, err = kvs.NewReference(ls, entry.Reference())
		if err != nil {
			return nil, nil, err
		}
	}

	return history, act, nil
}

func (c *ControllerStruct) saveHistoryAndAct(ctx context.Context, history History, mtdt *map[string]string, act kvs.KeyValueStore) (swarm.Address, swarm.Address, error) {
	actRef, err := act.Save(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	err = history.Add(ctx, actRef, nil, mtdt)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, err
	}
	historyRef, err := history.Store(ctx)
	if err != nil {
		return swarm.ZeroAddress, swarm.ZeroAddress, err
	}

	return historyRef, actRef, nil
}

func (c *ControllerStruct) getGranteeList(ctx context.Context, ls file.LoadSaver, encryptedglRef swarm.Address, publisher *ecdsa.PublicKey) (gl GranteeList, err error) {
	if encryptedglRef.IsZero() {
		gl, err = NewGranteeList(ls)
		if err != nil {
			return nil, err
		}
	} else {
		granteeref, err := c.decryptRefForPublisher(publisher, encryptedglRef)
		if err != nil {
			return nil, err
		}

		gl, err = NewGranteeListReference(ctx, ls, granteeref)
		if err != nil {
			return nil, err
		}
	}

	return gl, nil
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
