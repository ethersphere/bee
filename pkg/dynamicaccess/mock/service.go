// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
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

type mockDacService struct {
	historyMap map[string]dynamicaccess.History
	refMap     map[string]swarm.Address
	acceptAll  bool
	publisher  string
	encrypter  encryption.Interface
	ls         file.LoadSaver
}

type optionFunc func(*mockDacService)

// Option is an option passed to a mock dynamicaccess Service.
type Option interface {
	apply(*mockDacService)
}

func (f optionFunc) apply(r *mockDacService) { f(r) }

// New creates a new mock dynamicaccess service.
func New(o ...Option) dynamicaccess.Service {
	storer := mockstorer.New()
	m := &mockDacService{
		historyMap: make(map[string]dynamicaccess.History),
		refMap:     make(map[string]swarm.Address),
		publisher:  "",
		encrypter:  encryption.New(encryption.Key("b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"), 0, uint32(0), sha3.NewLegacyKeccak256),
		ls:         loadsave.New(storer.ChunkStore(), storer.Cache(), requestPipelineFactory(context.Background(), storer.Cache(), false, redundancy.NONE)),
	}
	for _, v := range o {
		v.apply(m)
	}

	return m
}

// WithAcceptAll sets the mock to return fixed references on every call to DownloadHandler.
func WithAcceptAll() Option {
	return optionFunc(func(m *mockDacService) { m.acceptAll = true })
}

func WithHistory(h dynamicaccess.History, ref string) Option {
	return optionFunc(func(m *mockDacService) {
		m.historyMap = map[string]dynamicaccess.History{ref: h}
	})
}

func WithPublisher(ref string) Option {
	return optionFunc(func(m *mockDacService) {
		m.publisher = ref
		m.encrypter = encryption.New(encryption.Key(ref), 0, uint32(0), sha3.NewLegacyKeccak256)
	})
}

func (m *mockDacService) DownloadHandler(ctx context.Context, timestamp int64, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, error) {
	if m.acceptAll {
		return swarm.ParseHexAddress("36e6c1bbdfee6ac21485d5f970479fd1df458d36df9ef4e8179708ed46da557f")
	}

	publicKeyBytes := crypto.EncodeSecp256k1PublicKey(publisher)
	p := hex.EncodeToString(publicKeyBytes)
	if m.publisher != "" && m.publisher != p {
		return swarm.ZeroAddress, fmt.Errorf("incorrect publisher")
	}

	h, exists := m.historyMap[historyRootHash.String()]
	if !exists {
		return swarm.ZeroAddress, fmt.Errorf("history not found")
	}
	kvsRef, err := h.Lookup(ctx, timestamp)
	if kvsRef.Equal(swarm.ZeroAddress) || err != nil {
		return swarm.ZeroAddress, fmt.Errorf("kvs not found")
	}
	return m.refMap[encryptedRef.String()], nil
}

func (m *mockDacService) UploadHandler(ctx context.Context, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash *swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error) {
	historyRef, _ := swarm.ParseHexAddress("67bdf80a9bbea8eca9c8480e43fdceb485d2d74d5708e45144b8c4adacd13d9c")
	kvsRef, _ := swarm.ParseHexAddress("3339613565613837623134316665343461613630396333333237656364383934")
	if m.acceptAll {
		encryptedRef, _ := swarm.ParseHexAddress("fc4e9fe978991257b897d987bc4ff13058b66ef45a53189a0b4fe84bb3346396")
		return kvsRef, historyRef, encryptedRef, nil
	}
	var (
		h      dynamicaccess.History
		exists bool
	)
	now := time.Now().Unix()
	if historyRootHash != nil {
		historyRef = *historyRootHash
		h, exists = m.historyMap[historyRef.String()]
		if !exists {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, fmt.Errorf("history not found")
		}
		kvsRef, _ = h.Lookup(ctx, now)
	} else {
		h, _ = dynamicaccess.NewHistory(m.ls, nil)
		h.Add(ctx, kvsRef, &now)
		historyRef, _ = h.Store(ctx)
		m.historyMap[historyRef.String()] = h
	}
	if kvsRef.Equal(swarm.ZeroAddress) {
		return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, fmt.Errorf("kvs not found")
	}
	encryptedRef, _ := m.encrypter.Encrypt(reference.Bytes())
	m.refMap[(hex.EncodeToString(encryptedRef))] = reference
	return kvsRef, historyRef, swarm.NewAddress(encryptedRef), nil
}

func (m *mockDacService) Close() error {
	return nil
}

func (m *mockDacService) Grant(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error {
	return nil
}
func (m *mockDacService) Revoke(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error {
	return nil
}
func (m *mockDacService) Commit(ctx context.Context, granteesAddress swarm.Address, actRootHash swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, swarm.Address, error) {
	return swarm.ZeroAddress, swarm.ZeroAddress, nil
}
func (m *mockDacService) HandleGrantees(ctx context.Context, rootHash swarm.Address, publisher *ecdsa.PublicKey, addList, removeList []*ecdsa.PublicKey) error {
	return nil
}
func (m *mockDacService) GetGrantees(ctx context.Context, rootHash swarm.Address) ([]*ecdsa.PublicKey, error) {
	return nil, nil
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}

var _ dynamicaccess.Controller = (*mockDacService)(nil)
