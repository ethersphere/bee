// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/langos"
	"github.com/gorilla/mux"
)

// The size of buffer used for prefetching content with Langos when not using erasure coding
// Warning: This value influences the number of chunk requests and chunker join goroutines
// per file request.
// Recommended value is 8 or 16 times the io.Copy default buffer value which is 32kB, depending
// on the file size. Use lookaheadBufferSize() to get the correct buffer size for the request.
const (
	smallFileBufferSize = 8 * 32 * 1024
	largeFileBufferSize = 16 * 32 * 1024

	largeBufferFilesizeThreshold = 10 * 1000000 // ten megs
)

func lookaheadBufferSize(size int64) int {
	if size <= largeBufferFilesizeThreshold {
		return smallFileBufferSize
	}
	return largeFileBufferSize
}

func (s *Service) bzzUploadHandler(w http.ResponseWriter, r *http.Request) {
	span, logger, ctx := s.tracer.StartSpanFromContext(r.Context(), "post_bzz", s.logger.WithName("post_bzz").Build())
	defer span.Finish()

	headers := struct {
		ContentType    string           `map:"Content-Type,mimeMediaType" validate:"required"`
		BatchID        []byte           `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64           `map:"Swarm-Tag"`
		Pin            bool             `map:"Swarm-Pin"`
		Deferred       *bool            `map:"Swarm-Deferred-Upload"`
		Encrypt        bool             `map:"Swarm-Encrypt"`
		IsDir          bool             `map:"Swarm-Collection"`
		RLevel         redundancy.Level `map:"Swarm-Redundancy-Level"`
		Act            bool             `map:"Swarm-Act"`
		HistoryAddress swarm.Address    `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag      uint64
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)

	ctx = redundancy.SetLevelInContext(ctx, headers.RLevel)

	if deferred || headers.Pin {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			ext.LogError(span, err, olog.String("action", "tag.create"))
			return
		}
		span.SetTag("tagID", tag)
	}

	putter, err := s.newStamperPutter(ctx, putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Pin:      headers.Pin,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("putter failed", "error", err)
		logger.Error(nil, "putter failed")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		ext.LogError(span, err, olog.String("action", "new.StamperPutter"))
		return
	}

	ow := &cleanupOnErrWriter{
		ResponseWriter: w,
		onErr:          putter.Cleanup,
		logger:         logger,
	}

	if headers.IsDir || headers.ContentType == multiPartFormData {
		s.dirUploadHandler(ctx, logger, span, ow, r, putter, r.Header.Get(ContentTypeHeader), headers.Encrypt, tag, headers.RLevel, headers.Act, headers.HistoryAddress)
		return
	}
	s.fileUploadHandler(ctx, logger, span, ow, r, putter, headers.Encrypt, tag, headers.RLevel, headers.Act, headers.HistoryAddress)
}

// fileUploadResponse is returned when an HTTP request to upload a file is successful
type bzzUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

// fileUploadHandler uploads the file and its metadata supplied in the file body and
// the headers
func (s *Service) fileUploadHandler(
	ctx context.Context,
	logger log.Logger,
	span opentracing.Span,
	w http.ResponseWriter,
	r *http.Request,
	putter storer.PutterSession,
	encrypt bool,
	tagID uint64,
	rLevel redundancy.Level,
	act bool,
	historyAddress swarm.Address,
) {
	queries := struct {
		FileName string `map:"name" validate:"startsnotwith=/"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	p := requestPipelineFn(putter, encrypt, rLevel)

	// first store the file and get its reference
	fr, err := p(ctx, r.Body)
	if err != nil {
		logger.Debug("file store failed", "file_name", queries.FileName, "error", err)
		logger.Error(nil, "file store failed", "file_name", queries.FileName)
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, errFileStore)
		}
		ext.LogError(span, err, olog.String("action", "file.store"))
		return
	}

	// If filename is still empty, use the file hash as the filename
	if queries.FileName == "" {
		queries.FileName = fr.String()
		if err := s.validate.Struct(queries); err != nil {
			verr := &validationError{
				Entry: "file hash",
				Value: queries.FileName,
				Cause: err,
			}
			logger.Debug("invalid body filename", "error", verr)
			logger.Error(nil, "invalid body filename")
			jsonhttp.BadRequest(w, jsonhttp.StatusResponse{
				Message: "invalid body params",
				Code:    http.StatusBadRequest,
				Reasons: []jsonhttp.Reason{{
					Field: "file hash",
					Error: verr.Error(),
				}},
			})
			return
		}
	}

	factory := requestPipelineFactory(ctx, putter, encrypt, rLevel)
	l := loadsave.New(s.storer.ChunkStore(), s.storer.Cache(), factory)

	m, err := manifest.NewDefaultManifest(l, encrypt)
	if err != nil {
		logger.Debug("create manifest failed", "file_name", queries.FileName, "error", err)
		logger.Error(nil, "create manifest failed", "file_name", queries.FileName)
		switch {
		case errors.Is(err, manifest.ErrInvalidManifestType):
			jsonhttp.BadRequest(w, "create manifest failed")
		default:
			jsonhttp.InternalServerError(w, nil)
		}
		return
	}

	rootMetadata := map[string]string{
		manifest.WebsiteIndexDocumentSuffixKey: queries.FileName,
	}
	err = m.Add(ctx, manifest.RootPath, manifest.NewEntry(swarm.ZeroAddress, rootMetadata))
	if err != nil {
		logger.Debug("adding metadata to manifest failed", "file_name", queries.FileName, "error", err)
		logger.Error(nil, "adding metadata to manifest failed", "file_name", queries.FileName)
		jsonhttp.InternalServerError(w, "add metadata failed")
		return
	}

	fileMtdt := map[string]string{
		manifest.EntryMetadataContentTypeKey: r.Header.Get(ContentTypeHeader), // Content-Type has already been validated.
		manifest.EntryMetadataFilenameKey:    queries.FileName,
	}

	err = m.Add(ctx, queries.FileName, manifest.NewEntry(fr, fileMtdt))
	if err != nil {
		logger.Debug("adding file to manifest failed", "file_name", queries.FileName, "error", err)
		logger.Error(nil, "adding file to manifest failed", "file_name", queries.FileName)
		jsonhttp.InternalServerError(w, "add file failed")
		return
	}

	logger.Debug("info", "encrypt", encrypt, "file_name", queries.FileName, "hash", fr, "metadata", fileMtdt)

	manifestReference, err := m.Store(ctx)
	if err != nil {
		logger.Debug("manifest store failed", "file_name", queries.FileName, "error", err)
		logger.Error(nil, "manifest store failed", "file_name", queries.FileName)
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "manifest store failed")
		}
		return
	}
	logger.Debug("store", "manifest_reference", manifestReference)

	reference := manifestReference
	if act {
		reference, err = s.actEncryptionHandler(r.Context(), w, putter, reference, historyAddress)
		if err != nil {
			jsonhttp.InternalServerError(w, errActUpload)
			return
		}
	}

	err = putter.Done(manifestReference)
	if err != nil {
		logger.Debug("done split failed", "reference", manifestReference, "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(w, "done split failed")
		ext.LogError(span, err, olog.String("action", "putter.Done"))
		return
	}
	span.LogFields(olog.Bool("success", true))
	span.SetTag("root_address", reference)

	if tagID != 0 {
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tagID))
		span.SetTag("tagID", tagID)
	}
	w.Header().Set(ETagHeader, fmt.Sprintf("%q", reference.String()))
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)

	jsonhttp.Created(w, bzzUploadResponse{
		Reference: reference,
	})
}

func (s *Service) bzzDownloadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_bzz_by_path").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
		Path    string        `map:"path"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	if strings.HasSuffix(paths.Path, "/") {
		paths.Path = strings.TrimRight(paths.Path, "/") + "/" // NOTE: leave one slash if there was some.
	}

	s.serveReference(logger, address, paths.Path, w, r, false)
}

func (s *Service) bzzHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("head_bzz_by_path").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
		Path    string        `map:"path"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	if strings.HasSuffix(paths.Path, "/") {
		paths.Path = strings.TrimRight(paths.Path, "/") + "/" // NOTE: leave one slash if there was some.
	}

	s.serveReference(logger, address, paths.Path, w, r, true)
}

func (s *Service) serveReference(logger log.Logger, address swarm.Address, pathVar string, w http.ResponseWriter, r *http.Request, headerOnly bool) {
	loggerV1 := logger.V(1).Build()

	headers := struct {
		Cache                 *bool            `map:"Swarm-Cache"`
		Strategy              *getter.Strategy `map:"Swarm-Redundancy-Strategy"`
		FallbackMode          *bool            `map:"Swarm-Redundancy-Fallback-Mode"`
		ChunkRetrievalTimeout *string          `map:"Swarm-Chunk-Retrieval-Timeout"`
	}{}

	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	cache := true
	if headers.Cache != nil {
		cache = *headers.Cache
	}

	ls := loadsave.NewReadonly(s.storer.Download(cache))
	feedDereferenced := false

	ctx := r.Context()
	ctx, err := getter.SetConfigInContext(ctx, headers.Strategy, headers.FallbackMode, headers.ChunkRetrievalTimeout, logger)
	if err != nil {
		logger.Error(err, err.Error())
		jsonhttp.BadRequest(w, "could not parse headers")
		return
	}

FETCH:
	// read manifest entry
	m, err := manifest.NewDefaultManifestReference(
		address,
		ls,
	)
	if err != nil {
		logger.Debug("bzz download: not manifest", "address", address, "error", err)
		logger.Error(nil, "not manifest")
		jsonhttp.NotFound(w, nil)
		return
	}

	// there's a possible ambiguity here, right now the data which was
	// read can be an entry.Entry or a mantaray feed manifest. Try to
	// unmarshal as mantaray first and possibly resolve the feed, otherwise
	// go on normally.
	if !feedDereferenced {
		if l, err := s.manifestFeed(ctx, m); err == nil {
			//we have a feed manifest here
			ch, cur, _, err := l.At(ctx, time.Now().Unix(), 0)
			if err != nil {
				logger.Debug("bzz download: feed lookup failed", "error", err)
				logger.Error(nil, "bzz download: feed lookup failed")
				jsonhttp.NotFound(w, "feed not found")
				return
			}
			if ch == nil {
				logger.Debug("bzz download: feed lookup: no updates")
				logger.Error(nil, "bzz download: feed lookup")
				jsonhttp.NotFound(w, "no update found")
				return
			}
			ref, _, err := parseFeedUpdate(ch)
			if err != nil {
				logger.Debug("bzz download: mapStructure feed update failed", "error", err)
				logger.Error(nil, "bzz download: mapStructure feed update failed")
				jsonhttp.InternalServerError(w, "mapStructure feed update")
				return
			}
			address = ref
			feedDereferenced = true
			curBytes, err := cur.MarshalBinary()
			if err != nil {
				s.logger.Debug("bzz download: marshal feed index failed", "error", err)
				s.logger.Error(nil, "bzz download: marshal index failed")
				jsonhttp.InternalServerError(w, "marshal index")
				return
			}

			w.Header().Set(SwarmFeedIndexHeader, hex.EncodeToString(curBytes))
			// this header might be overriding others. handle with care. in the future
			// we should implement an append functionality for this specific header,
			// since different parts of handlers might be overriding others' values
			// resulting in inconsistent headers in the response.
			w.Header().Set("Access-Control-Expose-Headers", SwarmFeedIndexHeader)
			goto FETCH
		}
	}

	if pathVar == "" {
		loggerV1.Debug("bzz download: handle empty path", "address", address)

		if indexDocumentSuffixKey, ok := manifestMetadataLoad(ctx, m, manifest.RootPath, manifest.WebsiteIndexDocumentSuffixKey); ok {
			pathWithIndex := path.Join(pathVar, indexDocumentSuffixKey)
			indexDocumentManifestEntry, err := m.Lookup(ctx, pathWithIndex)
			if err == nil {
				// index document exists
				logger.Debug("bzz download: serving path", "path", pathWithIndex)

				s.serveManifestEntry(logger, w, r, indexDocumentManifestEntry, !feedDereferenced, headerOnly)
				return
			}
		}
		logger.Debug("bzz download: address not found or incorrect", "address", address, "path", pathVar)
		logger.Error(nil, "address not found or incorrect")
		jsonhttp.NotFound(w, "address not found or incorrect")
		return
	}
	me, err := m.Lookup(ctx, pathVar)
	if err != nil {
		loggerV1.Debug("bzz download: invalid path", "address", address, "path", pathVar, "error", err)
		logger.Error(nil, "bzz download: invalid path")

		if errors.Is(err, manifest.ErrNotFound) {

			if !strings.HasPrefix(pathVar, "/") {
				// check for directory
				dirPath := pathVar + "/"
				exists, err := m.HasPrefix(ctx, dirPath)
				if err == nil && exists {
					// redirect to directory
					u := r.URL
					u.Path += "/"
					redirectURL := u.String()

					logger.Debug("bzz download: redirecting failed", "url", redirectURL, "error", err)

					http.Redirect(w, r, redirectURL, http.StatusPermanentRedirect)
					return
				}
			}

			// check index suffix path
			if indexDocumentSuffixKey, ok := manifestMetadataLoad(ctx, m, manifest.RootPath, manifest.WebsiteIndexDocumentSuffixKey); ok {
				if !strings.HasSuffix(pathVar, indexDocumentSuffixKey) {
					// check if path is directory with index
					pathWithIndex := path.Join(pathVar, indexDocumentSuffixKey)
					indexDocumentManifestEntry, err := m.Lookup(ctx, pathWithIndex)
					if err == nil {
						// index document exists
						logger.Debug("bzz download: serving path", "path", pathWithIndex)

						s.serveManifestEntry(logger, w, r, indexDocumentManifestEntry, !feedDereferenced, headerOnly)
						return
					}
				}
			}

			// check if error document is to be shown
			if errorDocumentPath, ok := manifestMetadataLoad(ctx, m, manifest.RootPath, manifest.WebsiteErrorDocumentPathKey); ok {
				if pathVar != errorDocumentPath {
					errorDocumentManifestEntry, err := m.Lookup(ctx, errorDocumentPath)
					if err == nil {
						// error document exists
						logger.Debug("bzz download: serving path", "path", errorDocumentPath)

						s.serveManifestEntry(logger, w, r, errorDocumentManifestEntry, !feedDereferenced, headerOnly)
						return
					}
				}
			}

			jsonhttp.NotFound(w, "path address not found")
		} else {
			jsonhttp.NotFound(w, nil)
		}
		return
	}

	// serve requested path
	s.serveManifestEntry(logger, w, r, me, !feedDereferenced, headerOnly)
}

func (s *Service) serveManifestEntry(
	logger log.Logger,
	w http.ResponseWriter,
	r *http.Request,
	manifestEntry manifest.Entry,
	etag, headersOnly bool,
) {
	additionalHeaders := http.Header{}
	mtdt := manifestEntry.Metadata()
	if fname, ok := mtdt[manifest.EntryMetadataFilenameKey]; ok {
		fname = filepath.Base(fname) // only keep the file name
		additionalHeaders[ContentDispositionHeader] =
			[]string{fmt.Sprintf("inline; filename=\"%s\"", fname)}
	}
	if mimeType, ok := mtdt[manifest.EntryMetadataContentTypeKey]; ok {
		additionalHeaders[ContentTypeHeader] = []string{mimeType}
	}

	s.downloadHandler(logger, w, r, manifestEntry.Reference(), additionalHeaders, etag, headersOnly)
}

// downloadHandler contains common logic for dowloading Swarm file from API
func (s *Service) downloadHandler(logger log.Logger, w http.ResponseWriter, r *http.Request, reference swarm.Address, additionalHeaders http.Header, etag, headersOnly bool) {
	headers := struct {
		Strategy              *getter.Strategy `map:"Swarm-Redundancy-Strategy"`
		FallbackMode          *bool            `map:"Swarm-Redundancy-Fallback-Mode"`
		ChunkRetrievalTimeout *string          `map:"Swarm-Chunk-Retrieval-Timeout"`
		LookaheadBufferSize   *int             `map:"Swarm-Lookahead-Buffer-Size"`
		Cache                 *bool            `map:"Swarm-Cache"`
	}{}

	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	cache := true
	if headers.Cache != nil {
		cache = *headers.Cache
	}

	ctx := r.Context()
	ctx, err := getter.SetConfigInContext(ctx, headers.Strategy, headers.FallbackMode, headers.ChunkRetrievalTimeout, logger)
	if err != nil {
		logger.Error(err, err.Error())
		jsonhttp.BadRequest(w, "could not parse headers")
		return
	}

	reader, l, err := joiner.New(ctx, s.storer.Download(cache), s.storer.Cache(), reference)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, topology.ErrNotFound) {
			logger.Debug("api download: not found ", "address", reference, "error", err)
			logger.Error(nil, err.Error())
			jsonhttp.NotFound(w, nil)
			return
		}
		logger.Debug("api download: unexpected error", "address", reference, "error", err)
		logger.Error(nil, "api download: unexpected error")
		jsonhttp.InternalServerError(w, "joiner failed")
		return
	}

	// include additional headers
	for name, values := range additionalHeaders {
		w.Header().Set(name, strings.Join(values, "; "))
	}
	if etag {
		w.Header().Set(ETagHeader, fmt.Sprintf("%q", reference))
	}
	w.Header().Set(ContentLengthHeader, strconv.FormatInt(l, 10))
	w.Header().Set("Access-Control-Expose-Headers", ContentDispositionHeader)

	if headersOnly {
		w.WriteHeader(http.StatusOK)
		return
	}

	bufSize := lookaheadBufferSize(l)
	if headers.LookaheadBufferSize != nil {
		bufSize = *(headers.LookaheadBufferSize)
	}
	if bufSize > 0 {
		http.ServeContent(w, r, "", time.Now(), langos.NewBufferedLangos(reader, bufSize))
		return
	}
	http.ServeContent(w, r, "", time.Now(), reader)
}

// manifestMetadataLoad returns the value for a key stored in the metadata of
// manifest path, or empty string if no value is present.
// The ok result indicates whether value was found in the metadata.
func manifestMetadataLoad(
	ctx context.Context,
	manifest manifest.Interface,
	path, metadataKey string,
) (string, bool) {
	me, err := manifest.Lookup(ctx, path)
	if err != nil {
		return "", false
	}

	manifestRootMetadata := me.Metadata()
	if val, ok := manifestRootMetadata[metadataKey]; ok {
		return val, ok
	}

	return "", false
}

func (s *Service) manifestFeed(
	ctx context.Context,
	m manifest.Interface,
) (feeds.Lookup, error) {
	e, err := m.Lookup(ctx, "/")
	if err != nil {
		return nil, fmt.Errorf("node lookup: %w", err)
	}
	var (
		owner, topic []byte
		t            = new(feeds.Type)
	)
	meta := e.Metadata()
	if e := meta[feedMetadataEntryOwner]; e != "" {
		owner, err = hex.DecodeString(e)
		if err != nil {
			return nil, err
		}
	}
	if e := meta[feedMetadataEntryTopic]; e != "" {
		topic, err = hex.DecodeString(e)
		if err != nil {
			return nil, err
		}
	}
	if e := meta[feedMetadataEntryType]; e != "" {
		err := t.FromString(e)
		if err != nil {
			return nil, err
		}
	}
	if len(owner) == 0 || len(topic) == 0 {
		return nil, fmt.Errorf("node lookup: %s", "feed metadata absent")
	}
	f := feeds.New(topic, common.BytesToAddress(owner))
	return s.feedFactory.NewLookup(*t, f)
}
