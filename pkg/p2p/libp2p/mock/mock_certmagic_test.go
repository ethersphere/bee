// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/mock"
	ma "github.com/multiformats/go-multiaddr"
)

func TestMockP2PForgeCertMgr_Start(t *testing.T) {
	certLoaded := make(chan struct{})
	mgr := mock.NewMockP2PForgeCertMgr(func() {
		close(certLoaded)
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() error = %v, wantErr %v", err, nil)
	}

	select {
	case <-certLoaded:
	case <-time.After(time.Second):
		t.Fatal("onCertLoaded callback was not triggered")
	}

	// Test idempotency
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() second call error = %v, wantErr %v", err, nil)
	}
}

func TestMockP2PForgeCertMgr_Stop(t *testing.T) {
	mgr := mock.NewMockP2PForgeCertMgr(nil)

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() error = %v, wantErr %v", err, nil)
	}

	mgr.Stop()

	// Test idempotency
	mgr.Stop()
}

func TestMockP2PForgeCertMgr_TLSConfig(t *testing.T) {
	mgr := mock.NewMockP2PForgeCertMgr(nil)
	cfg := mgr.TLSConfig()

	if cfg == nil {
		t.Fatal("TLSConfig() returned nil")
	}

	if len(cfg.Certificates) == 0 {
		t.Error("TLSConfig() returned no certificates")
	}
}

func TestMockP2PForgeCertMgr_AddressFactory(t *testing.T) {
	mgr := mock.NewMockP2PForgeCertMgr(nil)
	factory := mgr.AddressFactory()

	addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	addrs := []ma.Multiaddr{addr}

	got := factory(addrs)
	if len(got) != 1 || !got[0].Equal(addr) {
		t.Errorf("AddressFactory() = %v, want %v", got, addrs)
	}
}

func TestMockP2PForgeCertMgr_SetOnCertLoaded(t *testing.T) {
	mgr := mock.NewMockP2PForgeCertMgr(nil)
	certLoaded := make(chan struct{})

	mgr.SetOnCertLoaded(func() {
		close(certLoaded)
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() error = %v, wantErr %v", err, nil)
	}

	select {
	case <-certLoaded:
	case <-time.After(time.Second):
		t.Fatal("onCertLoaded callback was not triggered after SetOnCertLoaded")
	}
}

func TestMockP2PForgeCertMgr_GetCache(t *testing.T) {
	mgr := mock.NewMockP2PForgeCertMgr(nil)
	if mgr.GetCache() == nil {
		t.Error("GetCache() returned nil")
	}
}

func TestMockConfig(t *testing.T) {
	cfg := mock.NewMockConfig()
	tlsCfg := cfg.TLSConfig()
	if tlsCfg == nil {
		t.Fatal("TLSConfig() returned nil")
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("TLSConfig().MinVersion = %v, want %v", tlsCfg.MinVersion, tls.VersionTLS12)
	}
}
