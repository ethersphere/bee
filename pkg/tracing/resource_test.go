// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// attrValue returns the string value of key in res, or ("", false) when absent.
func attrValue(t *testing.T, res interface{ Set() *attribute.Set }, key attribute.Key) (string, bool) {
	t.Helper()

	v, ok := res.Set().Value(key)
	if !ok {
		return "", false
	}
	return v.AsString(), true
}

func TestNewResource(t *testing.T) {
	// Not parallel: a subtest uses t.Setenv, which forbids a parallel parent.
	// The parallel subtests below still pause until this function returns, by
	// which point the env-mutating subtest has run inline and restored the env.

	t.Run("service name and version", func(t *testing.T) {
		t.Parallel()

		res, err := tracing.NewResource(&tracing.Options{
			ServiceName:    "bee",
			ServiceVersion: "2.8.0-abcdef",
			Environment:    "mainnet",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, ok := attrValue(t, res, semconv.ServiceNameKey); !ok || got != "bee" {
			t.Errorf("service.name = %q (present=%v), want %q", got, ok, "bee")
		}
		if got, ok := attrValue(t, res, semconv.ServiceVersionKey); !ok || got != "2.8.0-abcdef" {
			t.Errorf("service.version = %q (present=%v), want %q", got, ok, "2.8.0-abcdef")
		}
		if got, ok := attrValue(t, res, semconv.DeploymentEnvironmentKey); !ok || got != "mainnet" {
			t.Errorf("deployment.environment = %q (present=%v), want %q", got, ok, "mainnet")
		}
		// WithTelemetrySDK must contribute the SDK identity attributes.
		if _, ok := attrValue(t, res, semconv.TelemetrySDKNameKey); !ok {
			t.Error("telemetry.sdk.name is absent, WithTelemetrySDK not applied")
		}
		// WithHost must contribute host.name so spans are attributable per node.
		if _, ok := attrValue(t, res, semconv.HostNameKey); !ok {
			t.Error("host.name is absent, WithHost not applied")
		}
	})

	t.Run("empty environment omits attribute", func(t *testing.T) {
		t.Parallel()

		res, err := tracing.NewResource(&tracing.Options{ServiceName: "bee"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, ok := attrValue(t, res, semconv.DeploymentEnvironmentKey); ok {
			t.Errorf("deployment.environment = %q, want it to be absent", got)
		}
	})

	t.Run("empty version omits attribute", func(t *testing.T) {
		t.Parallel()

		res, err := tracing.NewResource(&tracing.Options{ServiceName: "bee"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, ok := attrValue(t, res, semconv.ServiceVersionKey); ok {
			t.Errorf("service.version = %q, want it to be absent", got)
		}
	})

	// Configured service name/version are applied after WithFromEnv, so they
	// take precedence over a colliding OTEL_SERVICE_NAME while unrelated
	// environment attributes still enrich the resource. This subtest mutates the
	// process environment, so it must not run in parallel.
	t.Run("configured values win over env, env enriches", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "from-env")
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=testnet")

		res, err := tracing.NewResource(&tracing.Options{ServiceName: "configured"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, ok := attrValue(t, res, semconv.ServiceNameKey); !ok || got != "configured" {
			t.Errorf("service.name = %q (present=%v), want configured value to win", got, ok)
		}
		if got, ok := attrValue(t, res, semconv.DeploymentEnvironmentKey); !ok || got != "testnet" {
			t.Errorf("deployment.environment = %q (present=%v), want %q from env", got, ok, "testnet")
		}
	})
}
