// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package libp2p

import (
	"context"
	"crypto/tls"
	"sync"

	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
)

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
	// Paste the content of your cert.pem and key.pem files here
	const certPEM = `-----BEGIN CERTIFICATE-----
MIIDCzCCAfOgAwIBAgIUYmVQGLnuPXQb8yC4X69ll6FQjWQwDQYJKoZIhvcNAQEL
BQAwFDESMBAGA1UEAwwJbG9jYWxob3N0MCAXDTI1MDgwMTA3MzA0OFoYDzIxMjUw
NzA4MDczMDQ4WjAUMRIwEAYDVQQDDAlsb2NhbGhvc3QwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQC/NbDVAd4OdVMNiK+BjAFyTCIiJssgxhi6rWEgzykV
6lv0Q4/loUCwp1ylh6LJa6yjioGmxXl2CqIALcxRXg/DopJFgfIPPhghSHVE6AvV
PGf+Qw85vXfdjX6qymPsRRz5mMj9N9BidL1HgJjxpGw+7aDwXxMCvxwFN46kyfb9
fawIIlAJv2gxBoKxf+8AXwimPUrXxG1HziMPNTjQGW+zVRzeK4pTNdt+CXcf4vLM
LKv7eRXYiAL7m/6/UE/SQOXDGxvjkOTWgVX2gcV9tMZOaFJBp/WycsexiajRRxLQ
tl1weQhyBmLA7N494sYpwmzF1feybkkrPeU7Vo7T2gLrAgMBAAGjUzBRMB0GA1Ud
DgQWBBRs/abpJGBaT14jL05Fj9PVbc7T4TAfBgNVHSMEGDAWgBRs/abpJGBaT14j
L05Fj9PVbc7T4TAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQA1
mhrBSvtlW4mhxQAAEfX/JotieeGuG9nUE75LJ+sW5mRDO+5PZeHAajBdWFGftCm2
vNONQMVkAt8PSPuZ+6t284HSpWLTgYGTu19m9JFDfw3Dj7HIX95c0xq1WdNSDvpR
kRfoe7yc4B6mCKIU4EjAv0lUs21I2oLIbqQdhJneivutyCYcV/hxt7OJZs92g9HX
S8MGeWRCFhoPoOQ9tvszqzf7AB3uqiRuFxk2DkuZ7yN63/+DGj2kTBkf3Qe+4DyS
7DK5XkPdl/ajTXNoLdgYUw/5hdsbcTjnxRr37eKqoa/MtQxzQjoA5HWYoaDX+Vzp
OwmvrIFPPZ+bU+6+Xs5i
-----END CERTIFICATE-----`

	const keyPEM = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC/NbDVAd4OdVMN
iK+BjAFyTCIiJssgxhi6rWEgzykV6lv0Q4/loUCwp1ylh6LJa6yjioGmxXl2CqIA
LcxRXg/DopJFgfIPPhghSHVE6AvVPGf+Qw85vXfdjX6qymPsRRz5mMj9N9BidL1H
gJjxpGw+7aDwXxMCvxwFN46kyfb9fawIIlAJv2gxBoKxf+8AXwimPUrXxG1HziMP
NTjQGW+zVRzeK4pTNdt+CXcf4vLMLKv7eRXYiAL7m/6/UE/SQOXDGxvjkOTWgVX2
gcV9tMZOaFJBp/WycsexiajRRxLQtl1weQhyBmLA7N494sYpwmzF1feybkkrPeU7
Vo7T2gLrAgMBAAECggEAAlN0carm2Q7pUAgAVX3d4SvUyhEZNdCNw1MNeqgP0IUi
a22y1uyjjC9i4PdputYh+3bqgS1JdpmB6botbiQY0XZkyGQCt9fAB9yvjZRmylCz
b0mNe23l+lo2lvPimLGxmrTjZUwbfCR1m0Jupqoi4TqOud7nRohxwIQ6Zjmm3Y53
Ybiv8h46mhy2NZFI2zwaO9q0MpYozCsCR5Xi6DasJZedlAB+185mTJo79e2AllPI
Y3tsQLgrCc2i9TAoEjvJdCxNDuElusabKgROj3DPr2LEkApeX2EdknNKqMIb+NIk
7htDSYjoJuG1ABRB0vBx7s0OGtk+IC/xYwdYB8wgdQKBgQDszNAHHUGbBr2yh8MP
JXqcoxXiDMOjyrrJcTzkUGXP2tCG05u+do8e4Or1K6WXKJGuITeewd6cW/HE2Mlt
sc58b+6+H3hm1dkuAOj+uF6mV8Al2TxVoRrKlAojAzu9QhkH+9GYR+nGqz7wKSnS
6EQETiCfd5LYrK0OH12wOla+JQKBgQDOtpLWroD5Vjf4KJ2qX4un2V2IKyWMcofH
t4qWbD+6F4Kt/ZCxFKviLBwRffGaatp7E1W0kU76oflsrnpRzSzMearlLIIid8nA
ucXCuyJdQPSivsHJJM6MMQ4BzT2stR8JDtJMNip/JUb0pZFS/aG1BCp+nLSoU23m
q7vyUvJHzwKBgQDcaCaY+Jo/+Z5HtiXQy0m80e9kYA0ZP3FsXoIW4N5jAYBmfj/Q
n/nG/AK2ANI4SAKQ2Uoz8q+JSetXFZEnEQDowiatwA0JarKjJyW3MVSn77VhhTmr
WjDdrb1hqXjJR+SUkcccvpLR4ELMtwO+04G7oBytUVbVZqQNKRTDGwnyIQKBgD+t
1KxX05l76wACmxdqGZ6agoq5J/cNLTDkJMhUDomoRnSNAW7bvFuPVRI6Zxw3wJhb
i3J1tQvWq/zD/yCGAT/4VyIERQ6TMk6xq+9iMKLjqLkd5JqvQQXE8tixPkefADGN
JFGf+hVzCVnCS3NyeMdHwkOAyNJ16Qw/aUWsMcDXAoGBAJpIdS7fu7Odxp76ssjy
AUsUFvGLH3rndYUWANy9xmz5FAxt4pZAbtbIYqPXy+4hpXWhqgOmTti8GDqL1W8g
CdYKvjVOHj5GNMQ6QJ3b1MDmUOeSgaN7NIY7SuKlKT54nYkplej9cd/4h1DpjHar
3eEDaJABlw0eDY7F2qXJZzZ3
-----END PRIVATE KEY-----`

	// Use tls.X509KeyPair to create a certificate from the hardcoded strings
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		// This should not fail if the strings are pasted correctly
		// In a real test, you might use t.Fatalf("failed to load hardcoded cert: %v", err)
		return nil
	}

	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func (m *MockP2PForgeCertMgr) AddressFactory() config.AddrsFactory {
	return func(addrs []ma.Multiaddr) []ma.Multiaddr {
		return addrs
	}
}

func (m *MockP2PForgeCertMgr) GetCache() *MockCache {
	return m.cache
}
