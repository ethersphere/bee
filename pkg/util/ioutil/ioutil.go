// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"errors"
	"os"
	"path/filepath"
)

// DB folders paths from bee datadir
const (
	DataPathLocalstore = "localstore"
	DataPathKademlia   = "kademlia-metrics"
)

// The WriterFunc type is an adapter to allow the use of
// ordinary functions as io.Writer Write method. If f is
// a function with the appropriate signature, WriterFunc(f)
// is an io.Writer that calls f.
type WriterFunc func([]byte) (int, error)

// WriterFunc calls f(p).
func (f WriterFunc) Write(p []byte) (n int, err error) {
	return f(p)
}

// RemoveContent removes all files in path. Copied function from cmd/db.go
func RemoveContent(path string) error {
	dir, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()

	subpaths, err := dir.Readdirnames(0)
	if err != nil {
		return err
	}

	for _, sub := range subpaths {
		err = os.RemoveAll(filepath.Join(path, sub))
		if err != nil {
			return err
		}
	}
	return nil
}
