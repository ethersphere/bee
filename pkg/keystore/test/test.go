// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/keystore"
)

// Service is a utility testing function that can be used to test
// implementations of the keystore.Service interface.
func Service(t *testing.T, s keystore.Service, edg keystore.EDG) {
	t.Helper()

	exists, err := s.Exists("swarm")
	if err != nil {
		t.Fatal(err)
	}

	if exists {
		t.Fatal("should not exist")
	}

	// create a new swarm key
	k1, created, err := s.Key("swarm", "pass123456", edg)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("key is not created")
	}

	exists, err = s.Exists("swarm")
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatal("should exist")
	}

	// get swarm key
	k2, created, err := s.Key("swarm", "pass123456", edg)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("key is created, but should not be")
	}
	if !k1.Equal(k2) {
		t.Fatal("two keys are not equal")
	}

	// invalid password
	_, _, err = s.Key("swarm", "invalid password", edg)
	if !errors.Is(err, keystore.ErrInvalidPassword) {
		t.Fatal(err)
	}

	// create a new libp2p key
	k3, created, err := s.Key("libp2p", "p2p pass", edg)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("key is not created")
	}
	if k1.Equal(k3) {
		t.Fatal("two keys are equal, but should not be")
	}

	// get libp2p key
	k4, created, err := s.Key("libp2p", "p2p pass", edg)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("key is created, but should not be")
	}
	if !k3.Equal(k4) {
		t.Fatal("two keys are not equal")
	}
}
