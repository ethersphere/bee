// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package mock

import (
	"context"
	"crypto/tls"
	"sync"

	_ "embed"

	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
)

//go:embed testdata/cert.pem
var certPEM []byte

//go:embed testdata/key.pem
var keyPEM []byte

// MockFileStorage is a minimal implementation of certmagic.FileStorage for testing.
type MockFileStorage struct {
	path string
}

func NewMockFileStorage(path string) *MockFileStorage {
	return &MockFileStorage{path: path}
}

func (m *MockFileStorage) Store(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (m *MockFileStorage) Load(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (m *MockFileStorage) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *MockFileStorage) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *MockFileStorage) List(_ context.Context, _ string, _ bool) ([]string, error) {
	return nil, nil
}

func (m *MockFileStorage) Lock(_ context.Context, _ string) error {
	return nil
}

func (m *MockFileStorage) Unlock(_ context.Context, _ string) error {
	return nil
}

// MockCache is a minimal implementation of certmagic.Cache for testing.
type MockCache struct {
	stopped bool
	mu      sync.Mutex
}

func NewMockCache() *MockCache {
	return &MockCache{}
}

func (m *MockCache) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
}

// MockConfig is a minimal implementation of certmagic.Config for testing.
type MockConfig struct {
	cache *MockCache
}

func NewMockConfig() *MockConfig {
	return &MockConfig{cache: NewMockCache()}
}

func (m *MockConfig) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}

// MockP2PForgeCertMgr is a mock implementation of p2pforge.P2PForgeCertMgr.
type MockP2PForgeCertMgr struct {
	cache        *MockCache
	onCertLoaded func()
	started      bool
	mu           sync.Mutex
	ProvideHost  func(host.Host) error
}

func NewMockP2PForgeCertMgr(onCertLoaded func()) *MockP2PForgeCertMgr {
	return &MockP2PForgeCertMgr{
		cache:        NewMockCache(),
		onCertLoaded: onCertLoaded,
		ProvideHost:  func(_ host.Host) error { return nil },
	}
}

func (m *MockP2PForgeCertMgr) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}
	m.started = true
	if m.onCertLoaded != nil {
		go func() {
			m.onCertLoaded()
		}()
	}
	return nil
}
func (m *MockP2PForgeCertMgr) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return
	}
	m.started = false
	m.cache.Stop()
}

func (m *MockP2PForgeCertMgr) TLSConfig() *tls.Config {
	// Use tls.X509KeyPair to create a certificate from the hardcoded strings
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		// This should not fail if the strings are pasted correctly
		return nil
	}

	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func (m *MockP2PForgeCertMgr) SetOnCertLoaded(cb func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCertLoaded = cb
}

func (m *MockP2PForgeCertMgr) AddressFactory() config.AddrsFactory {
	return func(addrs []ma.Multiaddr) []ma.Multiaddr {
		return addrs
	}
}

func (m *MockP2PForgeCertMgr) GetCache() *MockCache {
	return m.cache
}
