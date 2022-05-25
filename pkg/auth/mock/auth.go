// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

type Auth struct {
	AuthorizeFunc   func(string) bool
	GenerateKeyFunc func(string) (string, error)
	EnforceFunc     func(string, string, string) (bool, error)
}

func (ma *Auth) Authorize(u string) bool {
	if ma.AuthorizeFunc == nil {
		return true
	}
	return ma.AuthorizeFunc(u)
}
func (ma *Auth) GenerateKey(k string, _ int) (string, error) {
	if ma.GenerateKeyFunc == nil {
		return "", nil
	}
	return ma.GenerateKeyFunc(k)
}
func (ma *Auth) RefreshKey(k string, _ int) (string, error) {
	if ma.GenerateKeyFunc == nil {
		return "", nil
	}
	return ma.GenerateKeyFunc(k)
}
func (ma *Auth) Enforce(a1 string, a2 string, a3 string) (bool, error) {
	return ma.EnforceFunc(a1, a2, a3)
}
