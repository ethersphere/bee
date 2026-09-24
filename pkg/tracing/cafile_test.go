// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestLoadCAFile(t *testing.T) {
	t.Parallel()

	t.Run("valid bundle", func(t *testing.T) {
		t.Parallel()

		path, caCert := writeTestCA(t)

		cfg, err := tracing.LoadCAFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.MinVersion != tls.VersionTLS12 {
			t.Errorf("MinVersion = %#x, want %#x", cfg.MinVersion, tls.VersionTLS12)
		}
		if cfg.RootCAs == nil {
			t.Fatal("RootCAs is nil")
		}
		// The loaded CA must be present in the pool: a self-signed CA verifies
		// against a root pool that actually contains it.
		if _, err := caCert.Verify(x509.VerifyOptions{Roots: cfg.RootCAs}); err != nil {
			t.Errorf("CA does not verify against the loaded pool: %v", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		if _, err := tracing.LoadCAFile(filepath.Join(t.TempDir(), "does-not-exist.pem")); err == nil {
			t.Fatal("expected an error for a missing CA file, got nil")
		}
	})

	t.Run("no valid certificates", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "garbage.pem")
		if err := os.WriteFile(path, []byte("this is not a PEM certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := tracing.LoadCAFile(path); err == nil {
			t.Fatal("expected an error for a file with no valid PEM certificates, got nil")
		}
	})
}

// TestNewTracer_CAFile checks that the CA bundle is wired into both OTLP
// transports when TLS is enabled, and ignored when the exporter is insecure.
func TestNewTracer_CAFile(t *testing.T) {
	t.Parallel()

	validCA, _ := writeTestCA(t)
	missingCA := filepath.Join(t.TempDir(), "missing.pem")

	for _, proto := range []string{"http", "grpc"} {
		t.Run(proto+"/valid CA loads", func(t *testing.T) {
			t.Parallel()

			_, closer, err := tracing.NewTracer(&tracing.Options{
				Enabled:     true,
				Endpoint:    "127.0.0.1:1",
				ServiceName: "test",
				Protocol:    proto,
				CAFile:      validCA,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			testutil.CleanupCloser(t, closer)
		})

		t.Run(proto+"/missing CA errors", func(t *testing.T) {
			t.Parallel()

			if _, _, err := tracing.NewTracer(&tracing.Options{
				Enabled:     true,
				Endpoint:    "127.0.0.1:1",
				ServiceName: "test",
				Protocol:    proto,
				CAFile:      missingCA,
			}); err == nil {
				t.Fatal("expected an error for a missing CA file, got nil")
			}
		})

		t.Run(proto+"/insecure ignores CA", func(t *testing.T) {
			t.Parallel()

			_, closer, err := tracing.NewTracer(&tracing.Options{
				Enabled:     true,
				Endpoint:    "127.0.0.1:1",
				ServiceName: "test",
				Protocol:    proto,
				Insecure:    true,
				CAFile:      missingCA,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			testutil.CleanupCloser(t, closer)
		})
	}
}

// writeTestCA generates a short-lived self-signed CA, writes it PEM-encoded to a
// temp file, and returns the path alongside the parsed certificate.
func writeTestCA(t *testing.T) (path string, caCert *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bee-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	caCert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	path = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}

	return path, caCert
}
