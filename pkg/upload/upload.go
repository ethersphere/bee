// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	multiPartFormData = "multipart/form-data"
	EncryptHeader     = "swarm-encrypt"
)

// FileUploadInfo contains the data for a file to be uploaded
type FileUploadInfo struct {
	fileName    string
	fileSize    int64
	contentType string
	toEncrypt   bool
	reader      io.Reader
}

// GetFileUploadInfo extracts file info for upload from HTTP request
func GetFileUploadInfo(r *http.Request) (*FileUploadInfo, error) {
	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse content type error: %v", err)
	}

	var reader io.Reader
	var fileName, contentLength string
	var fileSize uint64

	if mediaType == multiPartFormData {
		mr := multipart.NewReader(r.Body, params["boundary"])

		// read only the first part, as only one file upload is supported
		part, err := mr.NextPart()
		if err != nil {
			return nil, fmt.Errorf("read multipart error: %v", err)
		}

		// try to find filename
		// 1) in part header params
		// 2) as formname
		// 3) file reference hash (after uploading the file)
		if fileName = part.FileName(); fileName == "" {
			fileName = part.FormName()
		}

		// then find out content type
		contentType = part.Header.Get("Content-Type")
		if contentType == "" {
			br := bufio.NewReader(part)
			buf, err := br.Peek(512)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("read content type error: %v", err)
			}
			contentType = http.DetectContentType(buf)
			reader = br
		} else {
			reader = part
		}
		contentLength = part.Header.Get("Content-Length")
	} else {
		fileName = r.URL.Query().Get("name")
		contentLength = r.Header.Get("Content-Length")
		reader = r.Body
	}

	if contentLength != "" {
		fileSize, err = strconv.ParseUint(contentLength, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid content length error: %v", err)
		}
	} else {
		// copy the part to a tmp file to get its size
		tmp, err := ioutil.TempFile("", "bee-multipart")
		if err != nil {
			return nil, fmt.Errorf("create temp file error: %v", err)
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		n, err := io.Copy(tmp, reader)
		if err != nil {
			return nil, fmt.Errorf("write temp file error: %v", err)
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek temp file error: %v", err)
		}
		fileSize = uint64(n)
		reader = tmp
	}

	return &FileUploadInfo{
		fileName:    fileName,
		fileSize:    int64(fileSize),
		contentType: contentType,
		toEncrypt:   toEncrypt,
		reader:      reader,
	}, nil
}

// StoreFile stores the given file and returns its reference
func StoreFile(ctx context.Context, fileInfo *FileUploadInfo, s storage.Storer) (swarm.Address, error) {
	// first store the file and get its reference
	sp := splitter.NewSimpleSplitter(s)
	fr, err := file.SplitWriteAll(ctx, sp, fileInfo.reader, fileInfo.fileSize, fileInfo.toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split file error: %v", err)
	}

	// if filename is still empty, use the file hash as the filename
	if fileInfo.fileName == "" {
		fileInfo.fileName = fr.String()
	}

	// then store the metadata and get its reference
	m := entry.NewMetadata(fileInfo.fileName)
	m.MimeType = fileInfo.contentType
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("metadata marshal error: %v", err)
	}

	sp = splitter.NewSimpleSplitter(s)
	mr, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), fileInfo.toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split metadata error: %v", err)
	}

	// now join both references (mr, fr) to create an entry and store it
	e := entry.New(fr, mr)
	fileEntryBytes, err := e.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("entry marhsal error: %v", err)
	}
	sp = splitter.NewSimpleSplitter(s)
	reference, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), fileInfo.toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split entry error: %v", err)
	}

	return reference, nil
}
