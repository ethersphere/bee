// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/logging"
)

const (
	encryptionKey = "mZIODMvjsiS2VdK1xgI1cOTizhGVNoVz"
	passwordHash  = "$2a$12$mZIODMvjsiS2VdK1xgI1cOTizhGVNoVz2Xn48H8ddFFLzX2B3lD3m"
)

func TestAuthorize(t *testing.T) {
	a, err := auth.New(encryptionKey, passwordHash, nil)
	if err != nil {
		t.Error(err)
	}

	tt := []struct {
		desc     string
		pass     string
		expected bool
	}{
		{
			desc:     "correct credentials",
			pass:     "test",
			expected: true,
		}, {
			desc:     "wrong password",
			pass:     "notTest",
			expected: false,
		},
	}
	for _, tC := range tt {
		t.Run(tC.desc, func(t *testing.T) {
			res := a.Authorize(tC.pass)
			if res != tC.expected {
				t.Error("unexpected result", res)
			}
		})
	}
}

func TestExpiry(t *testing.T) {
	a, err := auth.New(encryptionKey, passwordHash, logging.New(io.Discard, 0))
	if err != nil {
		t.Error(err)
	}

	key, err := a.GenerateKey("role0", 1)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	time.Sleep(2 * time.Second)

	result, err := a.Enforce(key, "/bytes/1", "GET")
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Errorf("expected token expired error, got: %v", err)
	}

	if result {
		t.Errorf("expected %v, got %v", false, result)
	}
}

func TestEnforce(t *testing.T) {
	a, err := auth.New(encryptionKey, passwordHash, nil)
	if err != nil {
		t.Error(err)
	}

	tt := []struct {
		desc                   string
		role, resource, action string
		expected               bool
	}{
		{
			desc:     "success",
			role:     "role2",
			resource: "/pingpong/someone",
			action:   "POST",
			expected: true,
		}, {
			desc:     "success with query param",
			role:     "role1",
			resource: "/bzz?name=some-name",
			action:   "POST",
			expected: true,
		},
		{
			desc:     "bad role",
			role:     "role0",
			resource: "/pingpong/some-other-peer",
			action:   "POST",
		},
		{
			desc:     "bad resource",
			role:     "role2",
			resource: "/i-dont-exist",
			action:   "POST",
		},
		{
			desc:     "bad action",
			role:     "role2",
			resource: "/pingpong/someone",
			action:   "DELETE",
		},
	}

	for _, tC := range tt {
		t.Run(tC.desc, func(t *testing.T) {
			apiKey, err := a.GenerateKey(tC.role, 1)

			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			result, err := a.Enforce(apiKey, tC.resource, tC.action)

			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if result != tC.expected {
				t.Errorf("request from user with %s on object %s: expected %v, got %v", tC.role, tC.resource, tC.expected, result)
			}
		})
	}
}
