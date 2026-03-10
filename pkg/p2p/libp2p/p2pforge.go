// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/caddyserver/certmagic"
	"github.com/ethersphere/bee/v2/pkg/log"
	p2pforge "github.com/ipshipyard/p2p-forge/client"
	"github.com/libp2p/go-libp2p/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// P2PForgeOptions contains the configuration for creating a P2P Forge certificate manager.
type P2PForgeOptions struct {
	Domain               string
	RegistrationEndpoint string
	CAEndpoint           string
	StorageDir           string
}

// P2PForgeCertManager wraps the p2p-forge certificate manager with its associated zap logger.
type P2PForgeCertManager struct {
	certMgr   *p2pforge.P2PForgeCertMgr
	zapLogger *zap.Logger
}

// newP2PForgeCertManager creates a new P2P Forge certificate manager.
// It handles the creation of the storage directory and configures logging
// to match bee's verbosity level.
func newP2PForgeCertManager(beeLogger log.Logger, opts P2PForgeOptions) (*P2PForgeCertManager, error) {
	zapLogger, err := newZapLogger(beeLogger)
	if err != nil {
		return nil, fmt.Errorf("create zap logger: %w", err)
	}

	// Use storage dir with domain subdir for easier management of different registries.
	storagePath := filepath.Join(opts.StorageDir, opts.Domain)
	if err := os.MkdirAll(storagePath, 0o700); err != nil {
		return nil, fmt.Errorf("create certificate storage directory %s: %w", storagePath, err)
	}

	certMgr, err := p2pforge.NewP2PForgeCertMgr(
		p2pforge.WithForgeDomain(opts.Domain),
		p2pforge.WithForgeRegistrationEndpoint(opts.RegistrationEndpoint),
		p2pforge.WithCAEndpoint(opts.CAEndpoint),
		p2pforge.WithCertificateStorage(&certmagic.FileStorage{Path: storagePath}),
		p2pforge.WithLogger(zapLogger.Sugar()),
		p2pforge.WithUserAgent(userAgent()),
		p2pforge.WithAllowPrivateForgeAddrs(),
		p2pforge.WithRegistrationDelay(0),
		p2pforge.WithOnCertLoaded(func() {
			beeLogger.Info("auto tls certificate is loaded")
		}),
		p2pforge.WithOnCertRenewed(func() {
			beeLogger.Info("auto tls certificate is renewed")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize P2P Forge: %w", err)
	}

	return &P2PForgeCertManager{
		certMgr:   certMgr,
		zapLogger: zapLogger,
	}, nil
}

// CertMgr returns the underlying p2pforge.P2PForgeCertMgr.
func (m *P2PForgeCertManager) CertMgr() *p2pforge.P2PForgeCertMgr {
	return m.certMgr
}

// ZapLogger returns the zap logger used by the certificate manager.
func (m *P2PForgeCertManager) ZapLogger() *zap.Logger {
	return m.zapLogger
}

// autoTLSCertManager defines the interface for managing TLS certificates.
type autoTLSCertManager interface {
	Start() error
	Stop()
	TLSConfig() *tls.Config
	AddressFactory() config.AddrsFactory
}

// newZapLogger creates a zap logger configured to match bee's verbosity level.
// This is used by third-party libraries (like p2p-forge) that require a zap logger.
func newZapLogger(beeLogger log.Logger) (*zap.Logger, error) {
	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(beeVerbosityToZapLevel(beeLogger.Verbosity())),
		Development:      false,
		Encoding:         "json",
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.EpochTimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
	}
	return cfg.Build()
}

// beeVerbosityToZapLevel converts bee's log verbosity level to zap's log level.
func beeVerbosityToZapLevel(v log.Level) zapcore.Level {
	switch {
	case v <= log.VerbosityNone:
		return zap.FatalLevel // effectively silences the logger
	case v == log.VerbosityError:
		return zap.ErrorLevel
	case v == log.VerbosityWarning:
		return zap.WarnLevel
	case v == log.VerbosityInfo:
		return zap.InfoLevel
	default:
		return zap.DebugLevel // VerbosityDebug and VerbosityAll
	}
}
