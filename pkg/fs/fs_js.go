//go:build js && wasm
// +build js,wasm

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"syscall/js"
	"time"
)

var _ os.FileInfo = fsFileInfo{}

// fsFileInfo is an implementation of os.FileInfo for JavaScript/Wasm.
// It's backed by fs.
type fsFileInfo struct {
	js.Value
	path string
}

// Name returns the base name of the file.
func (fi fsFileInfo) Name() string {
	return filepath.Base(fi.path)
}

// Size returns the length of the file in bytes.
func (fi fsFileInfo) Size() int64 {
	return int64(fi.Value.Get("size").Int())
}

// Mode returns the file mode bits.
func (fi fsFileInfo) Mode() os.FileMode {
	return os.FileMode(fi.Value.Get("size").Int())
}

// ModTime returns the modification time.
func (fi fsFileInfo) ModTime() time.Time {
	modifiedTimeString := fi.Value.Get("mtime").String()
	modifiedTime, err := time.Parse(time.RFC3339, modifiedTimeString)
	if err != nil {
		panic(fmt.Errorf("could not convert string mtime (%q) to time.Time: %s", modifiedTimeString, err.Error()))
	}
	return modifiedTime
}

// IsDir is an abbreviation for Mode().IsDir().
func (fi fsFileInfo) IsDir() bool {
	return fi.Value.Call("isDirectory").Bool()
}

// underlying data source (always returns nil for wasm/js).
func (fi fsFileInfo) Sys() interface{} {
	return nil
}

var _ OsFile = &fsFile{}

// fsFile is an implementation of osFile for JavaScript/Wasm. It's backed
// by fs.
type fsFile struct {
	// name is the name of the file (including path)
	path string
	// fd is a file descriptor used as a reference to the file.
	fd int
	// currOffset is the current value of the offset used for reading or writing.
	currOffset int64
}

// Stat returns the FileInfo structure describing file. If there is an error,
// it will be of type *PathError.
func (f fsFile) Stat() (os.FileInfo, error) {
	return fsStat(f.path)
}

// Read reads up to len(b) bytes from the File. It returns the number of bytes
// read and any error encountered. At end of file, Read returns 0, io.EOF.
func (f *fsFile) Read(b []byte) (n int, err error) {
	bytesRead, err := f.read(b, f.currOffset)
	if bytesRead == 0 {
		return 0, io.EOF
	}
	f.currOffset += int64(bytesRead)
	return bytesRead, nil
}

// ReadAt reads len(b) bytes from the File starting at byte offset off. It
// returns the number of bytes read and the error, if any. ReadAt always
// returns a non-nil error when n < len(b). At end of file, that error is
// io.EOF.
func (f fsFile) ReadAt(b []byte, off int64) (n int, err error) {
	bytesRead, err := f.read(b, off)
	if bytesRead < len(b) {
		return bytesRead, io.EOF
	}
	return bytesRead, nil
}

func (f fsFile) read(b []byte, off int64) (n int, err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	// JavaScript API expects a Uint8Array which we then convert into []byte.
	buffer := js.Global().Get("Uint8Array").New(len(b))
	rawBytesRead := jsReadSync(f.fd, buffer, 0, len(b), int(off))
	bytesRead := rawBytesRead.Int()
	for i := 0; i < bytesRead; i++ {
		b[i] = byte(buffer.Index(i).Int())
	}
	return bytesRead, nil
}

// Write writes len(b) bytes to the File. It returns the number of bytes
// written and an error, if any. Write returns a non-nil error when n !=
// len(b).
func (f *fsFile) Write(b []byte) (n int, err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	uint8arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(uint8arr, b)
	rawBytesWritten := jsWriteSync(f.fd, uint8arr, 0, len(b), int(f.currOffset))
	bytesWritten := rawBytesWritten.Int()
	f.currOffset += int64(bytesWritten)
	if bytesWritten != len(b) {
		return bytesWritten, io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return bytesWritten, err
	}
	return bytesWritten, nil
}

// Seek sets the offset for the next Read or Write on file to offset,
// interpreted according to whence: 0 means relative to the origin of the
// file, 1 means relative to the current offset, and 2 means relative to the
// end. It returns the new offset and an error, if any. The behavior of Seek
// on a file opened with O_APPEND is not specified.
func (f *fsFile) Seek(offset int64, whence int) (ret int64, err error) {
	switch whence {
	case io.SeekStart:
		f.currOffset = offset
		return f.currOffset, nil
	case io.SeekCurrent:
		f.currOffset += offset
		return f.currOffset, nil
	case io.SeekEnd:
		f.currOffset = -offset
		return f.currOffset, nil
	}
	return 0, fmt.Errorf("Seek: unexpected whence value: %d", whence)
}

// Sync commits the current contents of the file to stable storage. Typically,
// this means flushing the file system's in-memory copy of recently written
// data to disk.
func (f fsFile) Sync() (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	jsFsyncSync(f.fd)
	return nil
}

// Close closes the File, rendering it unusable for I/O. On files that support
// SetDeadline, any pending I/O operations will be canceled and return
// immediately with an error.
func (f fsFile) Close() (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	jsCloseSync(f.fd)
	return nil
}

func fsStat(path string) (fileInfo os.FileInfo, err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	rawFileInfo := jsStatSync(path)
	return fsFileInfo{Value: rawFileInfo, path: path}, nil
}

func osOpen(path string) (OsFile, error) {
	if isfsSupported() {
		return fsOpenFile(path, os.O_RDONLY, os.ModePerm)
	}
	return os.Open(path)
}

func fsOpenFile(path string, flag int, perm os.FileMode) (file OsFile, err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	jsFlag, err := toJSFlag(flag)
	if err != nil {
		return nil, err
	}
	rawFD := jsOpenSync(path, jsFlag, int(perm))
	return &fsFile{path: path, fd: rawFD.Int()}, nil
}

func toJSFlag(flag int) (string, error) {
	// O_APPEND takes precedence
	if flag&os.O_APPEND != 0 {
		return "a", nil
	}
	// O_CREATE + O_RDWR
	if flag&os.O_CREATE != 0 && flag&os.O_RDWR != 0 {
		return "w+", nil // create if not exist, read/write, truncate
	}
	// O_CREATE + O_WRONLY
	if flag&os.O_CREATE != 0 && flag&os.O_WRONLY != 0 {
		return "w", nil // create if not exist, write only, truncate
	}
	// O_RDWR (no create)
	if flag&os.O_RDWR != 0 {
		return "r+", nil // read/write, fail if not exist
	}
	// O_WRONLY (no create)
	if flag&os.O_WRONLY != 0 {
		return "w", nil // write only, truncate, fail if not exist
	}
	// O_RDONLY
	return "r", nil // read only
}

func Readdirnames(path string, n int) ([]string, error) {
	if isfsSupported() {
		return fsReaddirnames(path, n)
	}
	// In Go, this requires two steps. Open the dir, then call Readdirnames.
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return dir.Readdirnames(n)
}

func fsReaddirnames(path string, n int) ([]string, error) {
	rawNames := jsReaddirSync(path)
	length := rawNames.Get("length").Int()
	if n != 0 && length > n {
		// If n > 0, only return up to n names.
		length = n
	}
	names := make([]string, length)
	for i := 0; i < length; i++ {
		names[i] = rawNames.Index(i).String()
	}
	return names, nil
}

func osMkdirAll(path string, perm os.FileMode) error {
	if isfsSupported() {
		return fsMkdirAll(path, perm)
	}
	return os.MkdirAll(path, perm)
}

func fsMkdirAll(path string, perm os.FileMode) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	// Note: mkdirAll is not supported by fs so we have to manually create
	// each directory.
	names := strings.Split(path, string(os.PathSeparator))
	for i := range names {
		partialPath := filepath.Join(names[:i+1]...)
		if err := fsMkdir(partialPath, perm); err != nil {
			if os.IsExist(err) {
				// If the directory already exists, that's fine.
				continue
			}
		}
	}
	return nil
}

func fsMkdir(dir string, perm os.FileMode) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	jsMkdirSync(dir, int(perm))
	return nil
}

func fsRename(oldpath, newpath string) error {
	jsRenameSync(oldpath, newpath)
	return nil
}

// isfsSupported returns true if fs is supported. It does this by
// checking for the global "fs" object.
func isfsSupported() bool {
	return !reflect.DeepEqual(js.Global().Get("ZenFS"), js.Null()) && !reflect.DeepEqual(js.Global().Get("ZenFS"), js.Undefined())
}

// convertJSError converts an error returned by the fs API into a Go
// error. This is important because Go expects certain types of errors to be
// returned (e.g. ENOENT when a file doesn't exist) and programs often change
// their behavior depending on the type of error.
func convertJSError(err js.Error) error {
	if reflect.DeepEqual(err.Value, js.Undefined()) || reflect.DeepEqual(err.Value, js.Null()) {
		return nil // No error
	}
	// There is an error, check the code
	if code := err.Value.Get("code"); !reflect.DeepEqual(code, js.Undefined()) && !reflect.DeepEqual(code, js.Null()) {
		switch code.String() {
		case "ENOENT":
			return os.ErrNotExist
		case "EISDIR":
			return syscall.EISDIR
		case "EEXIST":
			return os.ErrExist
		}
	}
	return err
}

// Note: JavaScript doesn't have an flock syscall so we have to fake it. This
// won't work if another process tries to read/write to the same file. It only
// works in the context of this process, but is safe with multiple goroutines.

// locksMu protects access to readLocks and writeLocks
var locksMu = sync.Mutex{}

// readLocks is a map of path to the number of readers.
var readLocks = map[string]uint{}

// writeLocks keeps track of files which are locked for writing.
var writeLocks = map[string]struct{}{}

type fsFileLock struct {
	path     string
	readOnly bool
	file     OsFile
}

func isErrInvalid(err error) bool {
	if err == os.ErrInvalid {
		return true
	}
	// Go >= 1.8 returns *os.PathError instead
	if patherr, ok := err.(*os.PathError); ok && patherr.Err == syscall.EINVAL {
		return true
	}
	return false
}

func jsReadSync(fd int, buffer js.Value, offset, length, position int) js.Value {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("read", fd, buffer, offset, length, position, callback)
	return waitForCallbackResults(resultsChan, errChan)
}

func jsWriteSync(fd int, data js.Value, offset, length, position int) js.Value {
	return js.Global().Get("ZenFS").Call("writeSync", fd, data, offset, length, position)
}

func jsFsyncSync(fd int) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("fsync", fd, callback)
	waitForCallbackResults(resultsChan, errChan)
}

func jsCloseSync(fd int) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("close", fd, callback)
	waitForCallbackResults(resultsChan, errChan)
}

func jsStatSync(path string) js.Value {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("stat", path, callback)
	return waitForCallbackResults(resultsChan, errChan)
}

func jsOpenSync(path string, flags string, mode int) js.Value {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("open", path, flags, mode, callback)
	return waitForCallbackResults(resultsChan, errChan)
}

func jsUnlinkSync(path string) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("unlink", path, callback)
	waitForCallbackResults(resultsChan, errChan)
}

func jsReaddirSync(path string) js.Value {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("readdir", path, callback)
	return waitForCallbackResults(resultsChan, errChan)
}

func jsMkdirSync(path string, mode int) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("mkdir", path, mode, callback)
	waitForCallbackResults(resultsChan, errChan)
}

func jsRenameSync(oldPath string, newPath string) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	js.Global().Get("ZenFS").Call("rename", oldPath, newPath, callback)
	waitForCallbackResults(resultsChan, errChan)
}

func jsWriteFileSync(filename string, data []byte, perm os.FileMode) {
	callback, resultsChan, errChan := makeAutoReleaseCallback()
	uint8arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(uint8arr, data)
	opts := js.Global().Get("Object").New()
	opts.Set("mode", int(perm))
	js.Global().Get("ZenFS").Call("writeFile", filename, uint8arr, opts, callback)
	waitForCallbackResults(resultsChan, errChan)
}

// makeAutoReleaseCallback creates and returns a js.Func that can be used as a
// callback. The callback will be released immediately after being called. The
// callback assumes the JavaScript callback convention and accepts two
// arguments: (err, result). If err is not null or undefined, it will send err
// through errChan. Otherwise, it will send result through resultsChan.
func makeAutoReleaseCallback() (callback js.Func, resultsChan chan js.Value, errChan chan error) {
	resultsChan = make(chan js.Value, 1)
	errChan = make(chan error, 1)
	callback = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		defer callback.Release()
		go func() {
			if len(args) == 0 {
				resultsChan <- js.Undefined()
				return
			}
			err := args[0]
			if !reflect.DeepEqual(err, js.Undefined()) && !reflect.DeepEqual(err, js.Null()) {
				errChan <- js.Error{Value: err}
				return
			}
			if len(args) >= 2 {
				resultsChan <- args[1]
			} else {
				resultsChan <- js.Undefined()
			}
		}()
		return nil
	})
	return callback, resultsChan, errChan
}

// waitForCallbackResults blocks until receiving from either resultsChan or
// errChan. If it receives from resultsChan first, it will return the result. If
// it receives from errChan first, it will panic with the error.
func waitForCallbackResults(resultsChan chan js.Value, errChan chan error) js.Value {
	select {
	case result := <-resultsChan:
		return result
	case err := <-errChan:
		// Expected to be recovered up the call stack.
		panic(err)
	}
}

func writeFile(filename string, data []byte, perm os.FileMode) error {
	jsWriteFileSync(filename, data, perm)
	return nil
}

func osOpenFile(name string, flag int, perm os.FileMode) (OsFile, error) {
	if isfsSupported() {
		return fsOpenFile(name, flag, perm)
	}
	return os.OpenFile(name, flag, perm)
}

func (f *fsFile) WriteAt(b []byte, off int64) (n int, err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	uint8arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(uint8arr, b)
	rawBytesWritten := jsWriteSync(f.fd, uint8arr, 0, len(b), int(off))
	bytesWritten := rawBytesWritten.Int()
	if bytesWritten != len(b) {
		return bytesWritten, io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return bytesWritten, err
	}
	return bytesWritten, nil
}
func (f *fsFile) Truncate(size int64) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	js.Global().Get("ZenFS").Call("ftruncateSync", f.fd, size)
	return nil
}

func osRemove(name string) error {
	if isfsSupported() {
		return fsRemove(name)
	}
	return os.Remove(name)
}

func fsRemove(name string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if jsErr, ok := e.(js.Error); ok {
				err = convertJSError(jsErr)
			}
		}
	}()
	jsUnlinkSync(name)
	return nil
}

func (f *fsFile) WriteString(s string) (n int, err error) {
	return f.Write([]byte(s))
}
