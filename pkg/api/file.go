// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	defaultBufSize = 4096
)

const (
	multiPartFormData = "multipart/form-data"
	EncryptHeader     = "swarm-encrypt"
)

// fileUploadResponse is returned when an HTTP request to upload a file is successful
type fileUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

// fileUploadHandler uploads the file and its metadata supplied as:
// - multipart http message
// - other content types as complete file body
func (s *server) fileUploadHandler(w http.ResponseWriter, r *http.Request) {
	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		s.Logger.Debugf("file upload: parse content type header %q: %v", contentType, err)
		s.Logger.Errorf("file upload: parse content type header %q", contentType)
		jsonhttp.BadRequest(w, "invalid content-type header")
		return
	}

	var reader io.Reader
	var fileName, contentLength string
	var fileSize uint64

	tag, err := s.getOrCreateTag(r.Header.Get(TagHeaderUid))
	if err != nil {
		s.Logger.Debugf("file upload: get or create tag: %v", err)
		s.Logger.Error("file upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)

	if mediaType == multiPartFormData {
		mr := multipart.NewReader(r.Body, params["boundary"])

		// read only the first part, as only one file upload is supported
		part, err := mr.NextPart()
		if err != nil {
			s.Logger.Debugf("file upload: read multipart: %v", err)
			s.Logger.Error("file upload: read multipart")
			jsonhttp.BadRequest(w, "invalid multipart/form-data")
			return
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
				s.Logger.Debugf("file upload: read content type, file %q: %v", fileName, err)
				s.Logger.Errorf("file upload: read content type, file %q", fileName)
				jsonhttp.BadRequest(w, "error reading content type")
				return
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
			s.Logger.Debugf("file upload: content length, file %q: %v", fileName, err)
			s.Logger.Errorf("file upload: content length, file %q", fileName)
			jsonhttp.BadRequest(w, "invalid content length header")
			return
		}
	} else {
		// copy the part to a tmp file to get its size
		tmp, err := ioutil.TempFile("", "bee-multipart")
		if err != nil {
			s.Logger.Debugf("file upload: create temporary file: %v", err)
			s.Logger.Errorf("file upload: create temporary file")
			jsonhttp.InternalServerError(w, nil)
			return
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		n, err := io.Copy(tmp, reader)
		if err != nil {
			s.Logger.Debugf("file upload: write temporary file: %v", err)
			s.Logger.Error("file upload: write temporary file")
			jsonhttp.InternalServerError(w, nil)
			return
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			s.Logger.Debugf("file upload: seek to beginning of temporary file: %v", err)
			s.Logger.Error("file upload: seek to beginning of temporary file")
			jsonhttp.InternalServerError(w, nil)
			return
		}
		fileSize = uint64(n)
		reader = tmp
	}

	// first store the file and get its reference
	sp := splitter.NewSimpleSplitter(s.Storer)
	fr, err := file.SplitWriteAll(ctx, sp, reader, int64(fileSize), toEncrypt)
	if err != nil {
		s.Logger.Debugf("file upload: file store, file %q: %v", fileName, err)
		s.Logger.Errorf("file upload: file store, file %q", fileName)
		jsonhttp.InternalServerError(w, "could not store file data")
		return
	}

	// If filename is still empty, use the file hash as the filename
	if fileName == "" {
		fileName = fr.String()
	}

	// then store the metadata and get its reference
	m := entry.NewMetadata(fileName)
	m.MimeType = contentType
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		s.Logger.Debugf("file upload: metadata marshal, file %q: %v", fileName, err)
		s.Logger.Errorf("file upload: metadata marshal, file %q", fileName)
		jsonhttp.InternalServerError(w, "metadata marshal error")
		return
	}
	sp = splitter.NewSimpleSplitter(s.Storer)
	mr, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), toEncrypt)
	if err != nil {
		s.Logger.Debugf("file upload: metadata store, file %q: %v", fileName, err)
		s.Logger.Errorf("file upload: metadata store, file %q", fileName)
		jsonhttp.InternalServerError(w, "could not store metadata")
		return
	}

	// now join both references (mr,fr) to create an entry and store it.
	entrie := entry.New(fr, mr)
	fileEntryBytes, err := entrie.MarshalBinary()
	if err != nil {
		s.Logger.Debugf("file upload: entry marshal, file %q: %v", fileName, err)
		s.Logger.Errorf("file upload: entry marshal, file %q", fileName)
		jsonhttp.InternalServerError(w, "entry marshal error")
		return
	}
	sp = splitter.NewSimpleSplitter(s.Storer)
	reference, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), toEncrypt)
	if err != nil {
		s.Logger.Debugf("file upload: entry store, file %q: %v", fileName, err)
		s.Logger.Errorf("file upload: entry store, file %q", fileName)
		jsonhttp.InternalServerError(w, "could not store entry")
		return
	}

	tag.DoneSplit(reference)

	w.Header().Set("ETag", fmt.Sprintf("%q", reference.String()))
	w.Header().Set(TagHeaderUid, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderUid)
	jsonhttp.OK(w, fileUploadResponse{
		Reference: reference,
	})
}

// fileUploadInfo contains the data for a file to be uploaded
type fileUploadInfo struct {
	name        string // file name
	size        int64  // file size
	contentType string
	reader      io.Reader
}

// storeFile uploads the given file and returns its reference
// this function was extracted from `fileUploadHandler` and should eventually replace its current code
func storeFile(ctx context.Context, fileInfo *fileUploadInfo, s storage.Storer) (swarm.Address, error) {
	v := ctx.Value(toEncryptContextKey{})
	toEncrypt, _ := v.(bool) // default is false

	// first store the file and get its reference
	sp := splitter.NewSimpleSplitter(s)
	fr, err := file.SplitWriteAll(ctx, sp, fileInfo.reader, fileInfo.size, toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split file error: %w", err)
	}

	// if filename is still empty, use the file hash as the filename
	if fileInfo.name == "" {
		fileInfo.name = fr.String()
	}

	// then store the metadata and get its reference
	m := entry.NewMetadata(fileInfo.name)
	m.MimeType = fileInfo.contentType
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("metadata marshal error: %w", err)
	}

	sp = splitter.NewSimpleSplitter(s)
	mr, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split metadata error: %w", err)
	}

	// now join both references (mr, fr) to create an entry and store it
	e := entry.New(fr, mr)
	fileEntryBytes, err := e.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("entry marshal error: %w", err)
	}
	sp = splitter.NewSimpleSplitter(s)
	reference, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split entry error: %w", err)
	}

	return reference, nil
}

// fileDownloadHandler downloads the file given the entry's reference.
func (s *server) fileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("file download: parse file address %s: %v", addr, err)
		s.Logger.Errorf("file download: parse file address %s", addr)
		jsonhttp.BadRequest(w, "invalid file address")
		return
	}

	toDecrypt := len(address.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	targets := r.URL.Query().Get("targets")
	sctx.SetTargets(r.Context(), targets)

	// read entry.
	j := joiner.NewSimpleJoiner(s.Storer)
	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(r.Context(), j, address, buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("file download: read entry %s: %v", addr, err)
		s.Logger.Errorf("file download: read entry %s", addr)
		jsonhttp.NotFound(w, nil)
		return
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("file download: unmarshal entry %s: %v", addr, err)
		s.Logger.Errorf("file download: unmarshal entry %s", addr)
		jsonhttp.InternalServerError(w, "error unmarshaling entry")
		return
	}

	// If none match header is set always send the reply as not modified
	// TODO: when SOC comes, we need to revisit this concept
	noneMatchEtag := r.Header.Get("If-None-Match")
	if noneMatchEtag != "" {
		if e.Reference().Equal(address) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// Read metadata.
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(r.Context(), j, e.Metadata(), buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("file download: read metadata %s: %v", addr, err)
		s.Logger.Errorf("file download: read metadata %s", addr)
		jsonhttp.NotFound(w, nil)
		return
	}
	metaData := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metaData)
	if err != nil {
		s.Logger.Debugf("file download: unmarshal metadata %s: %v", addr, err)
		s.Logger.Errorf("file download: unmarshal metadata %s", addr)
		jsonhttp.InternalServerError(w, "error unmarshaling metadata")
		return
	}

	additionalHeaders := http.Header{
		"Content-Disposition": {fmt.Sprintf("inline; filename=\"%s\"", metaData.Filename)},
		"Content-Type":        {metaData.MimeType},
	}

	s.downloadHandler(w, r, e.Reference(), additionalHeaders)
}

// downloadHandler contains common logic for dowloading Swarm file from API
func (s *server) downloadHandler(
	w http.ResponseWriter,
	r *http.Request,
	reference swarm.Address,
	additionalHeaders http.Header,
) {

	targets := r.URL.Query().Get("targets")
	sctx.SetTargets(r.Context(), targets)
	ctx := r.Context()

	toDecrypt := len(reference.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	j := joiner.NewSimpleJoiner(s.Storer)

	// send the file data back in the response
	dataSize, err := j.Size(ctx, reference)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("api download: not found %s: %v", reference, err)
			s.Logger.Error("api download: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("api download: invalid root chunk %s: %v", reference, err)
		s.Logger.Error("api download: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		ctx := r.Context()
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			if err := pr.CloseWithError(err); err != nil {
				s.Logger.Debugf("api download: data join close %s: %v", reference, err)
				s.Logger.Errorf("api download: data join close %s", reference)
			}
		}
	}()

	go func() {
		_, err := file.JoinReadAll(r.Context(), j, reference, pw, toDecrypt)
		if err := pw.CloseWithError(err); err != nil {
			s.Logger.Debugf("api download: data join close %s: %v", reference, err)
			s.Logger.Errorf("api download: data join close %s", reference)
		}
	}()

	bpr := bufio.NewReader(pr)

	if b, err := bpr.Peek(defaultBufSize); err != nil && err != io.EOF && len(b) == 0 {
		s.Logger.Debugf("api download: data join %s: %v", reference, err)
		s.Logger.Errorf("api download: data join %s", reference)
		jsonhttp.NotFound(w, nil)
		return
	}

	// include additional headers
	for name, values := range additionalHeaders {
		var v string
		for _, value := range values {
			if v != "" {
				v += "; "
			}
			v += value
		}
		w.Header().Set(name, v)
	}

	w.Header().Set("ETag", fmt.Sprintf("%q", reference))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	w.Header().Set("Decompressed-Content-Length", fmt.Sprintf("%d", dataSize))
	w.Header().Set(TargetsRecoveryHeader, targets)
	if _, err = io.Copy(w, bpr); err != nil {
		s.Logger.Debugf("api download: data read %s: %v", reference, err)
		s.Logger.Errorf("api download: data read %s", reference)
	}
}
