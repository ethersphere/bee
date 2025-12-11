//go:build !js
// +build !js

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reservefs.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fs

import (
	"os"
)

// osOpen calls os.Open.
func osOpen(name string) (OsFile, error) {
	return os.Open(name)
}

func osOpenFile(name string, flag int, perm os.FileMode) (OsFile, error) {
	return os.OpenFile(name, flag, perm)
}

// osMkdirAll calls os.MkdirAll.
func osMkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func writeFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func fsStat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func osRemove(name string) error {
	return os.Remove(name)
}
