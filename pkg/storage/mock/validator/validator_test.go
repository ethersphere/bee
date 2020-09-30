// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package validator_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMockValidator(t *testing.T) {
	validAddr := swarm.NewAddress([]byte("foo"))
	invalidAddr := swarm.NewAddress([]byte("bar"))

	validContent := []byte("xyzzy")
	invalidContent := []byte("yzzyx")

	validator := validator.NewValidator(validAddr, validContent)

	ch := swarm.NewChunk(validAddr, validContent)
	if !validator.Validate(context.Background(), ch) {
		t.Fatalf("chunk '%v' should be valid", ch)
	}

	ch = swarm.NewChunk(invalidAddr, validContent)
	if validator.Validate(context.Background(), ch) {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}

	ch = swarm.NewChunk(validAddr, invalidContent)
	if validator.Validate(context.Background(), ch) {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}

	ch = swarm.NewChunk(invalidAddr, invalidContent)
	if validator.Validate(context.Background(), ch) {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}
}
