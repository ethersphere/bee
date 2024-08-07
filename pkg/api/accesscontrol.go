// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type addressKey struct{}

const granteeListEncrypt = true

// getAddressFromContext is a helper function to extract the address from the context.
func getAddressFromContext(ctx context.Context) swarm.Address {
	v, ok := ctx.Value(addressKey{}).(swarm.Address)
	if ok {
		return v
	}
	return swarm.ZeroAddress
}

// setAddressInContext sets the swarm address in the context.
func setAddressInContext(ctx context.Context, address swarm.Address) context.Context {
	return context.WithValue(ctx, addressKey{}, address)
}

// GranteesPatchRequest represents a request to patch the list of grantees.
type GranteesPatchRequest struct {
	// Addlist contains the list of grantees to add.
	Addlist []string `json:"add"`

	// Revokelist contains the list of grantees to revoke.
	Revokelist []string `json:"revoke"`
}

// GranteesPatchResponse represents the response structure for patching grantees.
type GranteesPatchResponse struct {
	// Reference represents the swarm address.
	Reference swarm.Address `json:"ref"`
	// HistoryReference represents the reference to the history of an access control entry.
	HistoryReference swarm.Address `json:"historyref"`
}

// GranteesPostRequest represents the request structure for adding grantees.
type GranteesPostRequest struct {
	// GranteeList represents the list of grantees to be saves on Swarm.
	GranteeList []string `json:"grantees"`
}

// GranteesPostResponse represents the response structure for adding grantees.
type GranteesPostResponse struct {
	// Reference represents the saved grantee list Swarm address.
	Reference swarm.Address `json:"ref"`
	// HistoryReference represents the reference to the history of an access control entry.
	HistoryReference swarm.Address `json:"historyref"`
}

// GranteesPatch represents a structure for modifying the list of grantees.
type GranteesPatch struct {
	// Addlist is a list of ecdsa.PublicKeys to be added to a grantee list.
	Addlist []*ecdsa.PublicKey
	// Revokelist is a list of ecdsa.PublicKeys to be removed from a grantee list
	Revokelist []*ecdsa.PublicKey
}

// actDecryptionHandler is a middleware that looks up and decrypts the given address,
// if the act headers are present.
func (s *Service) actDecryptionHandler() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := s.logger.WithName("act_decryption_handler").Build()
			paths := struct {
				Address swarm.Address `map:"address,resolve" validate:"required"`
			}{}
			if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
				response("invalid path params", logger, w)
				return
			}

			headers := struct {
				Timestamp      *int64           `map:"Swarm-Act-Timestamp"`
				Publisher      *ecdsa.PublicKey `map:"Swarm-Act-Publisher"`
				HistoryAddress *swarm.Address   `map:"Swarm-Act-History-Address"`
				Cache          *bool            `map:"Swarm-Cache"`
			}{}
			if response := s.mapStructure(r.Header, &headers); response != nil {
				response("invalid header params", logger, w)
				return
			}

			// Try to download the file wihtout decryption, if the act headers are not present
			if headers.Publisher == nil || headers.HistoryAddress == nil {
				h.ServeHTTP(w, r)
				return
			}

			timestamp := time.Now().Unix()
			if headers.Timestamp != nil {
				timestamp = *headers.Timestamp
			}

			cache := true
			if headers.Cache != nil {
				cache = *headers.Cache
			}
			ctx := r.Context()
			ls := loadsave.NewReadonly(s.storer.Download(cache))
			reference, err := s.accesscontrol.DownloadHandler(ctx, ls, paths.Address, headers.Publisher, *headers.HistoryAddress, timestamp)
			if err != nil {
				logger.Debug("access control download failed", "error", err)
				logger.Error(nil, "access control download failed")
				switch {
				case errors.Is(err, accesscontrol.ErrNotFound):
					jsonhttp.NotFound(w, "act or history entry not found")
				case errors.Is(err, accesscontrol.ErrInvalidTimestamp):
					jsonhttp.BadRequest(w, "invalid timestamp")
				case errors.Is(err, accesscontrol.ErrInvalidPublicKey) || errors.Is(err, accesscontrol.ErrSecretKeyInfinity):
					jsonhttp.BadRequest(w, "invalid public key")
				case errors.Is(err, accesscontrol.ErrUnexpectedType):
					jsonhttp.BadRequest(w, "failed to create history")
				default:
					jsonhttp.InternalServerError(w, errActDownload)
				}
				return
			}
			h.ServeHTTP(w, r.WithContext(setAddressInContext(ctx, reference)))
		})
	}
}

// actEncryptionHandler is a middleware that encrypts the given address using the publisher's public key,
// uploads the encrypted reference, history and kvs to the store.
func (s *Service) actEncryptionHandler(
	ctx context.Context,
	w http.ResponseWriter,
	putter storer.PutterSession,
	reference swarm.Address,
	historyRootHash swarm.Address,
) (swarm.Address, error) {
	publisherPublicKey := &s.publicKey
	ls := loadsave.New(s.storer.Download(true), s.storer.Cache(), requestPipelineFactory(ctx, putter, false, redundancy.NONE))
	storageReference, historyReference, encryptedReference, err := s.accesscontrol.UploadHandler(ctx, ls, reference, publisherPublicKey, historyRootHash)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	// only need to upload history and kvs if a new history is created,
	// meaning that the publisher uploaded to the history for the first time
	if !historyReference.Equal(historyRootHash) {
		err = putter.Done(storageReference)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("done split key-value store failed: %w", err)
		}
		err = putter.Done(historyReference)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("done split history failed: %w", err)
		}
	}

	w.Header().Set(SwarmActHistoryAddressHeader, historyReference.String())
	return encryptedReference, nil
}

// actListGranteesHandler is a middleware that decrypts the given address and returns the list of grantees,
// only the publisher is authorized to access the list.
func (s *Service) actListGranteesHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("act_list_grantees_handler").Build()
	paths := struct {
		GranteesAddress swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		Cache *bool `map:"Swarm-Cache"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	cache := true
	if headers.Cache != nil {
		cache = *headers.Cache
	}
	publisher := &s.publicKey
	ls := loadsave.NewReadonly(s.storer.Download(cache))
	grantees, err := s.accesscontrol.Get(r.Context(), ls, publisher, paths.GranteesAddress)
	if err != nil {
		logger.Debug("could not get grantees", "error", err)
		logger.Error(nil, "could not get grantees")
		jsonhttp.NotFound(w, "granteelist not found")
		return
	}
	granteeSlice := make([]string, len(grantees))
	for i, grantee := range grantees {
		granteeSlice[i] = hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(grantee))
	}
	jsonhttp.OK(w, granteeSlice)
}

// actGrantRevokeHandler is a middleware that makes updates to the list of grantees,
// only the publisher is authorized to perform this action.
func (s *Service) actGrantRevokeHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("act_grant_revoke_handler").Build()

	if r.Body == http.NoBody {
		logger.Error(nil, "request has no body")
		jsonhttp.BadRequest(w, errInvalidRequest)
		return
	}

	paths := struct {
		GranteesAddress swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		BatchID        []byte         `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64         `map:"Swarm-Tag"`
		Pin            bool           `map:"Swarm-Pin"`
		Deferred       *bool          `map:"Swarm-Deferred-Upload"`
		HistoryAddress *swarm.Address `map:"Swarm-Act-History-Address" validate:"required"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	historyAddress := swarm.ZeroAddress
	if headers.HistoryAddress != nil {
		historyAddress = *headers.HistoryAddress
	}

	var (
		tag      uint64
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)

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
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		logger.Debug("read request body failed", "error", err)
		logger.Error(nil, "read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	gpr := GranteesPatchRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &gpr)
		if err != nil {
			logger.Debug("unmarshal body failed", "error", err)
			logger.Error(nil, "unmarshal body failed")
			jsonhttp.InternalServerError(w, "error unmarshaling request body")
			return
		}
	}

	grantees := GranteesPatch{}
	parsedAddlist, err := parseKeys(gpr.Addlist)
	if err != nil {
		logger.Debug("add list key parse failed", "error", err)
		logger.Error(nil, "add list key parse failed")
		jsonhttp.BadRequest(w, "invalid add list")
		return
	}
	grantees.Addlist = append(grantees.Addlist, parsedAddlist...)

	parsedRevokelist, err := parseKeys(gpr.Revokelist)
	if err != nil {
		logger.Debug("revoke list key parse failed", "error", err)
		logger.Error(nil, "revoke list key parse failed")
		jsonhttp.BadRequest(w, "invalid revoke list")
		return
	}
	grantees.Revokelist = append(grantees.Revokelist, parsedRevokelist...)

	ctx := r.Context()
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
		return
	}

	granteeref := paths.GranteesAddress
	publisher := &s.publicKey
	ls := loadsave.New(s.storer.Download(true), s.storer.Cache(), requestPipelineFactory(ctx, putter, false, redundancy.NONE))
	gls := loadsave.New(s.storer.Download(true), s.storer.Cache(), requestPipelineFactory(ctx, putter, granteeListEncrypt, redundancy.NONE))
	granteeref, encryptedglref, historyref, actref, err := s.accesscontrol.UpdateHandler(ctx, ls, gls, granteeref, historyAddress, publisher, grantees.Addlist, grantees.Revokelist)
	if err != nil {
		logger.Debug("failed to update grantee list", "error", err)
		logger.Error(nil, "failed to update grantee list")
		switch {
		case errors.Is(err, accesscontrol.ErrNotFound):
			jsonhttp.NotFound(w, "act or history entry not found")
		case errors.Is(err, accesscontrol.ErrNoGranteeFound):
			jsonhttp.BadRequest(w, "remove from empty grantee list")
		case errors.Is(err, accesscontrol.ErrUnexpectedType):
			jsonhttp.BadRequest(w, "failed to create history")
		default:
			jsonhttp.InternalServerError(w, errActGranteeList)
		}
		return
	}

	err = putter.Done(actref)
	if err != nil {
		logger.Debug("done split act failed", "error", err)
		logger.Error(nil, "done split act failed")
		jsonhttp.InternalServerError(w, "done split act failed")
		return
	}

	err = putter.Done(historyref)
	if err != nil {
		logger.Debug("done split history failed", "error", err)
		logger.Error(nil, "done split history failed")
		jsonhttp.InternalServerError(w, "done split history failed")
		return
	}

	err = putter.Done(granteeref)
	if err != nil {
		logger.Debug("done split grantees failed", "error", err)
		logger.Error(nil, "done split grantees failed")
		jsonhttp.InternalServerError(w, "done split grantees failed")
		return
	}

	jsonhttp.OK(w, GranteesPatchResponse{
		Reference:        encryptedglref,
		HistoryReference: historyref,
	})
}

// actCreateGranteesHandler is a middleware that creates a new list of grantees,
// only the publisher is authorized to perform this action.
func (s *Service) actCreateGranteesHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("acthandler").Build()

	if r.Body == http.NoBody {
		logger.Error(nil, "request has no body")
		jsonhttp.BadRequest(w, errInvalidRequest)
		return
	}

	headers := struct {
		BatchID        []byte         `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64         `map:"Swarm-Tag"`
		Pin            bool           `map:"Swarm-Pin"`
		Deferred       *bool          `map:"Swarm-Deferred-Upload"`
		HistoryAddress *swarm.Address `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	historyAddress := swarm.ZeroAddress
	if headers.HistoryAddress != nil {
		historyAddress = *headers.HistoryAddress
	}

	var (
		tag      uint64
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)

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
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		logger.Debug("read request body failed", "error", err)
		logger.Error(nil, "read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	gpr := GranteesPostRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &gpr)
		if err != nil {
			logger.Debug("unmarshal body failed", "error", err)
			logger.Error(nil, "unmarshal body failed")
			jsonhttp.InternalServerError(w, "error unmarshaling request body")
			return
		}
	}

	list, err := parseKeys(gpr.GranteeList)
	if err != nil {
		logger.Debug("create list key parse failed", "error", err)
		logger.Error(nil, "create list key parse failed")
		jsonhttp.BadRequest(w, "invalid grantee list")
		return
	}

	ctx := r.Context()
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
		return
	}

	publisher := &s.publicKey
	ls := loadsave.New(s.storer.Download(true), s.storer.Cache(), requestPipelineFactory(ctx, putter, false, redundancy.NONE))
	gls := loadsave.New(s.storer.Download(true), s.storer.Cache(), requestPipelineFactory(ctx, putter, granteeListEncrypt, redundancy.NONE))
	granteeref, encryptedglref, historyref, actref, err := s.accesscontrol.UpdateHandler(ctx, ls, gls, swarm.ZeroAddress, historyAddress, publisher, list, nil)
	if err != nil {
		logger.Debug("failed to create grantee list", "error", err)
		logger.Error(nil, "failed to create grantee list")
		switch {
		case errors.Is(err, accesscontrol.ErrNotFound):
			jsonhttp.NotFound(w, "act or history entry not found")
		case errors.Is(err, accesscontrol.ErrUnexpectedType):
			jsonhttp.BadRequest(w, "failed to create history")
		default:
			jsonhttp.InternalServerError(w, errActGranteeList)
		}
		return
	}

	err = putter.Done(actref)
	if err != nil {
		logger.Debug("done split act failed", "error", err)
		logger.Error(nil, "done split act failed")
		jsonhttp.InternalServerError(w, "done split act failed")
		return
	}

	err = putter.Done(historyref)
	if err != nil {
		logger.Debug("done split history failed", "error", err)
		logger.Error(nil, "done split history failed")
		jsonhttp.InternalServerError(w, "done split history failed")
		return
	}

	err = putter.Done(granteeref)
	if err != nil {
		logger.Debug("done split grantees failed", "error", err)
		logger.Error(nil, "done split grantees failed")
		jsonhttp.InternalServerError(w, "done split grantees failed")
		return
	}

	jsonhttp.Created(w, GranteesPostResponse{
		Reference:        encryptedglref,
		HistoryReference: historyref,
	})
}

func parseKeys(list []string) ([]*ecdsa.PublicKey, error) {
	parsedList := make([]*ecdsa.PublicKey, 0, len(list))
	for _, g := range list {
		h, err := hex.DecodeString(g)
		if err != nil {
			return []*ecdsa.PublicKey{}, fmt.Errorf("failed to decode grantee: %w", err)
		}
		k, err := btcec.ParsePubKey(h)
		if err != nil {
			return []*ecdsa.PublicKey{}, fmt.Errorf("failed to parse grantee public key: %w", err)
		}
		parsedList = append(parsedList, k.ToECDSA())
	}

	return parsedList, nil
}
