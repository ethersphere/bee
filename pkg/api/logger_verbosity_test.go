// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// syncBuffer is a bytes.Buffer safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestSetLoggerVerbosityIsLogged checks that a verbosity change is logged
// before it is applied, so that turning logging off still leaves a record.
// nolint:paralleltest
func TestSetLoggerVerbosityIsLogged(t *testing.T) {
	defer func(fn api.LogSetVerbosityByExpFn) {
		api.ReplaceLogSetVerbosityByExp(fn)
	}(api.LogSetVerbosityByExp)

	sink := new(syncBuffer)
	client, _, _, _, _ := newTestServer(t, testServerOptions{
		Logger: log.NewLogger("test", log.WithSink(sink), log.WithVerbosity(log.VerbosityInfo)),
	})

	const exp = "^node/api$"
	var loggedBefore bool
	api.ReplaceLogSetVerbosityByExp(func(string, log.Level) error {
		loggedBefore = strings.Contains(sink.String(), "setting logger verbosity")
		return nil
	})

	url := "/loggers/" + base64.URLEncoding.EncodeToString([]byte(exp)) + "/none"
	jsonhttptest.Request(t, client, http.MethodPut, url, http.StatusOK)

	if !loggedBefore {
		t.Fatalf("verbosity change was not logged before being applied; log: %q", sink.String())
	}
	if !strings.Contains(sink.String(), `"verbosity"="none"`) {
		t.Fatalf("log does not record the new verbosity; log: %q", sink.String())
	}
}
