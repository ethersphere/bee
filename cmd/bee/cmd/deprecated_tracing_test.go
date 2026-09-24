// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeprecatedTracingKeysAccepted verifies that the pre-OpenTelemetry tracing
// keys are tolerated in a config file (so stale configs still boot) while a
// genuinely unknown key is still rejected.
func TestDeprecatedTracingKeysAccepted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		config    string
		wantError string
	}{
		{
			name: "deprecated tracing keys accepted",
			config: "tracing-enable: false\n" +
				"tracing-endpoint: my-jaeger:6831\n" +
				"tracing-host: \"\"\n" +
				"tracing-port: \"\"\n",
		},
		{
			name:      "unknown key rejected",
			config:    "totally-made-up-key: 1\n",
			wantError: "totally-made-up-key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfgFile := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(cfgFile, []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}

			c := newCommand(t)
			c.SetCfgFileForTest(cfgFile)
			startCmd := c.SubCommandForTest("start")
			if startCmd == nil {
				t.Fatal("start subcommand not found")
			}

			err := c.CheckUnknownParamsForTest(startCmd)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
		})
	}
}
