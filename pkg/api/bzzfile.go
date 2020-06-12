// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

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
	"strconv"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	MultiPartFormData = "multipart/form-data"
)

type FileUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bzzFileUploadHandler uploads the file and its metadata supplied as a multipart http message.
func (s *server) bzzFileUploadHandler(w http.ResponseWriter, r *http.Request) {
	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != MultiPartFormData {
		s.Logger.Debugf("bzz-file: no mutlipart: %v", err)
		s.Logger.Error("bzz-file: no mutlipart")
		jsonhttp.BadRequest(w, "not a mutlipart/form-data")
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			s.Logger.Debugf("bzz-file: read mutlipart: %v", err)
			s.Logger.Error("bzz-file: read mutlipart")
			jsonhttp.BadRequest(w, "error reading a mutlipart/form-data")
			return
		}

		ctx := context.Background()

		// find file size
		fileSizeString := part.Header.Get("Content-Length")
		if fileSizeString == "" {
			s.Logger.Debugf("bzz-file: content length: %v", err)
			s.Logger.Error("bzz-file: content length")
			jsonhttp.BadRequest(w, "content length header missing")
			return
		}
		fileSize, err := strconv.ParseUint(fileSizeString, 10, 64)
		if err != nil {
			s.Logger.Debugf("bzz-file: content length: %v", err)
			s.Logger.Error("bzz-file: content length")
			jsonhttp.BadRequest(w, "error parsing content length")
			return
		}

		// first store the file and get its reference
		fr, err := s.storePartData(ctx, part, fileSize)
		if err != nil {
			s.Logger.Debugf("bzz-file: file store: %v,", err)
			s.Logger.Error("bzz-file: file store")
			jsonhttp.InternalServerError(w, "could not store file data")
			return
		}

		// try to find filename
		// 1) in part header params
		// 2) as formname
		// 3) file reference hash
		fileName := part.FileName()
		if fileName == "" {
			fileName = part.FormName()
		}
		if fileName == "" {
			fileName = fr.String()
		}
		m := entry.NewMetadata(fileName)

		// then find out content type
		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			reader := bufio.NewReader(part)
			buf, err := reader.Peek(512)
			if err != nil {
				s.Logger.Debugf("bzz-file: read content type: %v, file name %s", err, fileName)
				s.Logger.Error("bzz-file: read content type")
				jsonhttp.BadRequest(w, "error reading content type")
				return
			}
			contentType = http.DetectContentType(buf)
		}
		m.MimeType = contentType

		// Store the metadata and get its reference
		metadataBytes, err := json.Marshal(m)
		if err != nil {
			s.Logger.Debugf("bzz-file: metadata marshall: %v, file name %s", err, fileName)
			s.Logger.Error("bzz-file: metadata marshall")
			jsonhttp.InternalServerError(w, "metadata marshall error")
			return
		}
		mr, err := s.storeMeta(ctx, metadataBytes)
		if err != nil {
			s.Logger.Debugf("bzz-file: metadata store: %v, file name %s", err, fileName)
			s.Logger.Error("bzz-file: metadata store")
			jsonhttp.InternalServerError(w, "could not store metadata")
			return
		}

		// now join both references (mr,fr) to create an entry and store it.
		entrie := entry.New(fr, mr)
		fileEntryBytes, err := entrie.MarshalBinary()
		if err != nil {
			s.Logger.Debugf("bzz-file: entry marshall: %v, file name %s", err, fileName)
			s.Logger.Error("bzz-file: entry marshall")
			jsonhttp.InternalServerError(w, "entry marshall error")
			return
		}
		er, err := s.storeMeta(ctx, fileEntryBytes)
		if err != nil {
			s.Logger.Debugf("bzz-file: entry store: %v, file name %s", err, fileName)
			s.Logger.Error("bzz-file: entry store")
			jsonhttp.InternalServerError(w, "could not store entry")
			return
		}

		w.Header().Set("ETag", fmt.Sprintf("%q", er.String()))
		jsonhttp.OK(w, &FileUploadResponse{Reference: er})
	}
}

// bzzFileDownloadHandler downloads the file given the entry's reference.
func (s *server) bzzFileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-file: parse file address %s: %v", addr, err)
		s.Logger.Error("bzz-file: parse file address")
		jsonhttp.BadRequest(w, "invalid file address")
		return
	}

	// read entry.
	j := joiner.NewSimpleJoiner(s.Storer)
	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(j, address, buf)
	if err != nil {
		s.Logger.Debugf("bzz-file: read entry %s: %v", addr, err)
		s.Logger.Error("bzz-file: read entry")
		jsonhttp.InternalServerError(w, "error reading entry")
		return
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz-file: unmarshall entry %s: %v", addr, err)
		s.Logger.Error("bzz-file: unmarshall entry")
		jsonhttp.InternalServerError(w, "error unmarshalling entry")
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
	_, err = file.JoinReadAll(j, e.Metadata(), buf)
	if err != nil {
		s.Logger.Debugf("bzz-file: read metadata %s: %v", addr, err)
		s.Logger.Error("bzz-file: read netadata")
		jsonhttp.InternalServerError(w, "error reading metadata")
		return
	}
	metaData := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metaData)
	if err != nil {
		s.Logger.Debugf("bzz-file: unmarshall metadata %s: %v", addr, err)
		s.Logger.Error("bzz-file: unmarshall metadata")
		jsonhttp.InternalServerError(w, "error unmarshalling metadata")
		return
	}

	// send the file data back in the response
	outBuffer := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(j, e.Reference(), outBuffer)
	if err != nil {
		s.Logger.Debugf("bzz-file: data read %s: %v", addr, err)
		s.Logger.Error("bzz-file: data read")
		jsonhttp.InternalServerError(w, "error reading data")
		return
	}
	w.Header().Set("ETag", fmt.Sprintf("%q", e.Reference()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", metaData.Filename))
	w.Header().Set("Content-Type", metaData.MimeType)
	_, _ = io.Copy(w, outBuffer)
}

// storeMeta is used to store metadata information as a whole.
func (s *server) storeMeta(ctx context.Context, dataBytes []byte) (swarm.Address, error) {
	dataBuf := bytes.NewBuffer(dataBytes)
	dataReader := io.LimitReader(dataBuf, int64(len(dataBytes)))
	dataReadCloser := ioutil.NopCloser(dataReader)
	o, err := s.splitUpload(ctx, dataReadCloser, int64(len(dataBytes)))
	if err != nil {
		return swarm.ZeroAddress, err
	}
	bytesResp := o.(bytesPostResponse)
	return bytesResp.Reference, nil
}

// storePartData stores file data belonging to one of the part of multipart.
func (s *server) storePartData(ctx context.Context, part *multipart.Part, l uint64) (swarm.Address, error) {
	o, err := s.splitUpload(ctx, part, int64(l))
	if err != nil {
		return swarm.ZeroAddress, err
	}
	bytesResp := o.(bytesPostResponse)
	return bytesResp.Reference, nil
}
