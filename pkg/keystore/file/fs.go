//go:build !js
// +build !js

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reservefs.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package file

import (
	"os"
)

// osOpen calls os.Open.
func osOpen(name string) (osFile, error) {
	return os.Open(name)
}

// Readdirnames opens the directory and calls Readdirnames on it.
func Readdirnames(dirname string, n int) (names []string, err error) {
	dir, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	return dir.Readdirnames(n)
}

// osMkdirAll calls os.MkdirAll.
func osMkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}
