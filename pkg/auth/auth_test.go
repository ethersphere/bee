// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/auth"
)

const oneHour = 1 * time.Hour

func TestAuthorize(t *testing.T) {
	a, err := auth.New("test", "test", oneHour)
	if err != nil {
		t.Error(err)
	}

	tt := []struct {
		desc       string
		user, pass string
		expected   bool
	}{
		{
			desc:     "correct credentials",
			user:     "test",
			pass:     "test",
			expected: true,
		}, {
			desc:     "wrong name",
			user:     "bad",
			pass:     "test",
			expected: false,
		}, {
			desc:     "wrong password",
			user:     "test",
			pass:     "bad",
			expected: false,
		},
	}
	for _, tC := range tt {
		t.Run(tC.desc, func(t *testing.T) {
			res := a.Authorize(tC.user, tC.pass)
			if res != tC.expected {
				t.Error("unexpected result", res)
			}
		})
	}
}

func TestEnforceWithNonExistentApiKey(t *testing.T) {
	a, err := auth.New("test", "test", oneHour)
	if err != nil {
		t.Error(err)
	}
	result := a.Enforce("non-existent", "/resource", "GET")

	if result {
		t.Errorf("expected %v, got %v", false, result)
	}
}

func TestExpiry(t *testing.T) {
	oneMili := 1 * time.Millisecond
	a, err := auth.New("test", "test", oneMili)
	if err != nil {
		t.Error(err)
	}
	key := a.AddKey("test", "role0")

	time.Sleep(oneMili)

	result := a.Enforce(key, "/bytes/1", "GET")

	if result {
		t.Errorf("expected %v, got %v", false, result)
	}
}

func TestEnforce(t *testing.T) {
	a, err := auth.New("test", "test", oneHour)
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
			apiKey := a.AddKey("test", tC.role)

			result := a.Enforce(apiKey, tC.resource, tC.action)

			if result != tC.expected {
				t.Errorf("request from user with %s on object %s: expected %v, got %v", tC.role, tC.resource, tC.expected, result)
			}
		})
	}
}
