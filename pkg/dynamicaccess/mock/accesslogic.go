// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package mock

type AccessLogicMock struct {
	GetFunc func(string, string, string) (string, error)
}

func (ma *AccessLogicMock) Get(encryped_ref string, publisher string, tag string) (string, error) {
	if ma.GetFunc == nil {
		return "", nil
	}
	return ma.GetFunc(encryped_ref, publisher, tag)
}
