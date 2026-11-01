// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type mockFailingSplitter struct{}

func (m *mockFailingSplitter) Split(ctx context.Context, r io.ReadCloser, l int64, toEncrypt bool) (swarm.Address, error) {
	return swarm.ZeroAddress, errors.New("splitter failure")
}

func TestSplitWriteAll_SplitError(t *testing.T) {
	t.Parallel()

	data := []byte("some payload data")
	r := bytes.NewReader(data)
	splitter := &mockFailingSplitter{}

	_, err := file.SplitWriteAll(context.Background(), splitter, r, int64(len(data)), false)
	if err == nil {
		t.Fatal("expected error from failing splitter")
	}
}
