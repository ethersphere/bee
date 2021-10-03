// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

type Auth struct {
	AuthorizeFunc func(string) bool
	AddKeyFunc    func(string) (string, error)
}

func (ma *Auth) Authorize(u string) bool {
	if ma.AuthorizeFunc == nil {
		return true
	}
	return ma.AuthorizeFunc(u)
}
func (ma *Auth) GenerateKey(k string, _ int) (string, error) {
	if ma.AddKeyFunc == nil {
		return "", nil
	}
	return ma.AddKeyFunc(k)
}
func (*Auth) Enforce(string, string, string) (bool, error) {
	return false, nil
}
