// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package validator_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMockValidator(t *testing.T) {
	validAddr := swarm.NewAddress([]byte("foo"))
	invalidAddr := swarm.NewAddress([]byte("bar"))

	validContent := []byte("xyzzy")
	invalidContent := []byte("yzzyx")

	validator := validator.NewMockValidator(validAddr, validContent)

	ch := swarm.NewChunk(validAddr, validContent)
	yes, _ := validator.Validate(ch)
	if !yes {
		t.Fatalf("chunk '%v' should be valid", ch)
	}

	ch = swarm.NewChunk(invalidAddr, validContent)
	yes, _ = validator.Validate(ch)
	if yes {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}

	ch = swarm.NewChunk(validAddr, invalidContent)
	yes, _ = validator.Validate(ch)
	if yes {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}

	ch = swarm.NewChunk(invalidAddr, invalidContent)
	yes, _ = validator.Validate(ch)
	if yes {
		t.Fatalf("chunk '%v' should be invalid", ch)
	}
}
