// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reservefs.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fs

import (
	"io"
	"os"
	"path/filepath"
)

// osFile is an interface for interacting with a file. In Go, the underlying
// type of osFile is always simply *os.File. In JavaScript/Wasm, the underlying
// type is a wrapper type which mimics the functionality of os.File.
type OsFile interface {
	// Stat returns the FileInfo structure describing file. If there is an error,
	// it will be of type *PathError.
	Stat() (os.FileInfo, error)
	// Read reads up to len(b) bytes from the File. It returns the number of bytes
	// read and any error encountered. At end of file, Read returns 0, io.EOF.
	Read(b []byte) (n int, err error)
	// ReadAt reads len(b) bytes from the File starting at byte offset off. It
	// returns the number of bytes read and the error, if any. ReadAt always
	// returns a non-nil error when n < len(b). At end of file, that error is
	// io.EOF.
	ReadAt(b []byte, off int64) (n int, err error)
	// Write writes len(b) bytes to the File. It returns the number of bytes
	// written and an error, if any. Write returns a non-nil error when n !=
	// len(b).
	Write(b []byte) (n int, err error)
	// Seek sets the offset for the next Read or Write on file to offset,
	// interpreted according to whence: 0 means relative to the origin of the
	// file, 1 means relative to the current offset, and 2 means relative to the
	// end. It returns the new offset and an error, if any. The behavior of Seek
	// on a file opened with O_APPEND is not specified.
	Seek(offset int64, whence int) (ret int64, err error)
	// Sync commits the current contents of the file to stable storage. Typically,
	// this means flushing the file system's in-memory copy of recently written
	// data to disk.
	Sync() error
	// Close closes the File, rendering it unusable for I/O. On files that support
	// SetDeadline, any pending I/O operations will be canceled and return
	// immediately with an error.
	Close() error

	WriteAt([]byte, int64) (int, error)
	Truncate(size int64) error
	WriteString(s string) (n int, err error)
}

func ReadFile(filename string) ([]byte, error) {
	f, err := osOpen(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

func MkdirAll(path string, perm os.FileMode) error {
	return osMkdirAll(path, perm)
}

func Open(name string) (OsFile, error) {
	return osOpen(name)
}

func OpenFile(name string, flag int, perm os.FileMode) (OsFile, error) {
	return osOpenFile(name, flag, perm)
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	if err := MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return writeFile(filename, data, perm)
}

func Stat(name string) (os.FileInfo, error) {
	return fsStat(name)
}

func Remove(name string) error {
	return osRemove(name)
}
