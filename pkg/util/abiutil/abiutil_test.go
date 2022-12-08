// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abiutil

import (
	"strings"
	"testing"
)

func TestMustParseABI(t *testing.T) {
	t.Parallel()

	defer func() {
		switch err := recover(); {
		case err == nil:
			t.Error("expected panic")
		case !strings.Contains(err.(error).Error(), "unable to parse ABI:"):
			t.Errorf("unexpected panic: %v", err)
		}
	}()

	MustParseABI("invalid abi")
}
