// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mock provides a mock implementation for the
// access control functionalities.
//
//nolint:ireturn
package mock

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

type mockController struct {
	historyMap map[string]accesscontrol.History
	refMap     map[string]swarm.Address
	acceptAll  bool
	publisher  string
	encrypter  encryption.Interface
	ls         file.LoadSaver
}

type optionFunc func(*mockController)

// Option is an option passed to a mock accesscontrol Controller.
type Option interface {
	apply(*mockController)
}

func (f optionFunc) apply(r *mockController) { f(r) }

// New creates a new mock accesscontrol Controller.
func New(o ...Option) accesscontrol.Controller {
	storer := mockstorer.New()
	m := &mockController{
		historyMap: make(map[string]accesscontrol.History),
		refMap:     make(map[string]swarm.Address),
		publisher:  "",
		encrypter:  encryption.New(encryption.Key("b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"), 0, 0, sha3.NewLegacyKeccak256),
		ls:         loadsave.New(storer.ChunkStore(), storer.Cache(), requestPipelineFactory(context.Background(), storer.Cache(), false, redundancy.NONE)),
	}
	for _, v := range o {
		v.apply(m)
	}

	return m
}

// WithAcceptAll sets the mock to return fixed references on every call to DownloadHandler.
func WithAcceptAll() Option {
	return optionFunc(func(m *mockController) { m.acceptAll = true })
}

// WithHistory sets the mock to use the given history reference.
func WithHistory(h accesscontrol.History, ref string) Option {
	return optionFunc(func(m *mockController) {
		m.historyMap = map[string]accesscontrol.History{ref: h}
	})
}

// WithPublisher sets the mock to use the given reference as the publisher address.
func WithPublisher(ref string) Option {
	return optionFunc(func(m *mockController) {
		m.publisher = ref
		m.encrypter = encryption.New(encryption.Key(ref), 0, 0, sha3.NewLegacyKeccak256)
	})
}

func (m *mockController) DownloadHandler(ctx context.Context, ls file.LoadSaver, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address, timestamp int64) (swarm.Address, error) {
	if m.acceptAll {
		return swarm.ParseHexAddress("36e6c1bbdfee6ac21485d5f970479fd1df458d36df9ef4e8179708ed46da557f")
	}

	publicKeyBytes := crypto.EncodeSecp256k1PublicKey(publisher)
	p := hex.EncodeToString(publicKeyBytes)
	if m.publisher != "" && m.publisher != p {
		return swarm.ZeroAddress, accesscontrol.ErrInvalidPublicKey
	}

	h, exists := m.historyMap[historyRootHash.String()]
	if !exists {
		return swarm.ZeroAddress, accesscontrol.ErrNotFound
	}
	entry, err := h.Lookup(ctx, timestamp)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	kvsRef := entry.Reference()
	if kvsRef.IsZero() {
		return swarm.ZeroAddress, err
	}
	return m.refMap[encryptedRef.String()], nil
}

func (m *mockController) UploadHandler(ctx context.Context, ls file.LoadSaver, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error) {
	historyRef, _ := swarm.ParseHexAddress("67bdf80a9bbea8eca9c8480e43fdceb485d2d74d5708e45144b8c4adacd13d9c")
	kvsRef, _ := swarm.ParseHexAddress("3339613565613837623134316665343461613630396333333237656364383934")
	if m.acceptAll {
		encryptedRef, _ := swarm.ParseHexAddress("fc4e9fe978991257b897d987bc4ff13058b66ef45a53189a0b4fe84bb3346396")
		return kvsRef, historyRef, encryptedRef, nil
	}
	var (
		h      accesscontrol.History
		exists bool
	)

	publicKeyBytes := crypto.EncodeSecp256k1PublicKey(publisher)
	p := hex.EncodeToString(publicKeyBytes)
	if m.publisher != "" && m.publisher != p {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, accesscontrol.ErrInvalidPublicKey
	}

	now := time.Now().Unix()
	if !historyRootHash.IsZero() {
		historyRef = historyRootHash
		h, exists = m.historyMap[historyRef.String()]
		if !exists {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, accesscontrol.ErrNotFound
		}
		entry, err := h.Lookup(ctx, now)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		kvsRef := entry.Reference()
		if kvsRef.IsZero() {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, fmt.Errorf("kvs not found")
		}
	} else {
		h, _ = accesscontrol.NewHistory(m.ls)
		_ = h.Add(ctx, kvsRef, &now, nil)
		historyRef, _ = h.Store(ctx)
		m.historyMap[historyRef.String()] = h
	}

	encryptedRef, _ := m.encrypter.Encrypt(reference.Bytes())
	m.refMap[(hex.EncodeToString(encryptedRef))] = reference
	return kvsRef, historyRef, swarm.NewAddress(encryptedRef), nil
}

func (m *mockController) Close() error {
	return nil
}

func (m *mockController) UpdateHandler(_ context.Context, ls file.LoadSaver, gls file.LoadSaver, encryptedglref swarm.Address, historyref swarm.Address, publisher *ecdsa.PublicKey, addList []*ecdsa.PublicKey, removeList []*ecdsa.PublicKey) (swarm.Address, swarm.Address, swarm.Address, swarm.Address, error) {
	if historyref.Equal(swarm.EmptyAddress) || encryptedglref.Equal(swarm.EmptyAddress) {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, accesscontrol.ErrNotFound
	}
	historyRef, _ := swarm.ParseHexAddress("67bdf80a9bbea8eca9c8480e43fdceb485d2d74d5708e45144b8c4adacd13d9c")
	glRef, _ := swarm.ParseHexAddress("3339613565613837623134316665343461613630396333333237656364383934")
	eglRef, _ := swarm.ParseHexAddress("fc4e9fe978991257b897d987bc4ff13058b66ef45a53189a0b4fe84bb3346396")
	actref, _ := swarm.ParseHexAddress("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")
	return glRef, eglRef, historyRef, actref, nil
}

func (m *mockController) Get(ctx context.Context, ls file.LoadSaver, publisher *ecdsa.PublicKey, encryptedglref swarm.Address) ([]*ecdsa.PublicKey, error) {
	if m.publisher == "" {
		return nil, fmt.Errorf("granteelist not found")
	}
	keys := []string{
		"a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa",
		"b786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfb",
		"c786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfc",
	}
	pubkeys := make([]*ecdsa.PublicKey, 0, len(keys))
	for i := range keys {
		data, err := hex.DecodeString(keys[i])
		if err != nil {
			panic(err)
		}

		privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
		pubKey := privKey.PublicKey
		if err != nil {
			panic(err)
		}
		pubkeys = append(pubkeys, &pubKey)
	}
	return pubkeys, nil
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}

var _ accesscontrol.Controller = (*mockController)(nil)
