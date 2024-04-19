package api

import (
	"context"
	"crypto/ecdsa"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type addressKey struct{}

// getAddressFromContext is a helper function to extract the address from the context
func getAddressFromContext(ctx context.Context) swarm.Address {
	v, ok := ctx.Value(addressKey{}).(swarm.Address)
	if ok {
		return v
	}
	return swarm.ZeroAddress
}

// setAddress sets the swarm address in the context
func setAddressInContext(ctx context.Context, address swarm.Address) context.Context {
	return context.WithValue(ctx, addressKey{}, address)
}

// actDecryptionHandler is a middleware that looks up and decrypts the given address,
// if the act headers are present
func (s *Service) actDecryptionHandler() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := s.logger.WithName("acthandler").Build()
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
			}{}
			if response := s.mapStructure(r.Header, &headers); response != nil {
				response("invalid header params", logger, w)
				return
			}

			// Try to download the file wihtout decryption, if the act headers are not present
			if headers.Publisher == nil || headers.Timestamp == nil || headers.HistoryAddress == nil {
				h.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			reference, err := s.dac.DownloadHandler(ctx, paths.Address, headers.Publisher, *headers.HistoryAddress, *headers.Timestamp)
			if err != nil {
				jsonhttp.InternalServerError(w, errActDownload)
				return
			}
			h.ServeHTTP(w, r.WithContext(setAddressInContext(ctx, reference)))
		})
	}

}

// actEncryptionHandler is a middleware that encrypts the given address using the publisher's public key
// Uploads the encrypted reference, history and kvs to the store
func (s *Service) actEncryptionHandler(
	ctx context.Context,
	logger log.Logger,
	w http.ResponseWriter,
	putter storer.PutterSession,
	reference swarm.Address,
	historyRootHash swarm.Address,
) (swarm.Address, error) {
	publisherPublicKey := &s.publicKey
	storageReference, historyReference, encryptedReference, err := s.dac.UploadHandler(ctx, reference, publisherPublicKey, historyRootHash)
	if err != nil {
		logger.Debug("act failed to encrypt reference", "error", err)
		logger.Error(nil, "act failed to encrypt reference")
		return swarm.ZeroAddress, err
	}
	// only need to upload history and kvs if a new history is created,
	// meaning that the publsher uploaded to the history for the first time
	if !historyReference.Equal(historyRootHash) {
		err = putter.Done(storageReference)
		if err != nil {
			logger.Debug("done split keyvaluestore failed", "error", err)
			logger.Error(nil, "done split keyvaluestore failed")
			return swarm.ZeroAddress, err
		}
		err = putter.Done(historyReference)
		if err != nil {
			logger.Debug("done split history failed", "error", err)
			logger.Error(nil, "done split history failed")
			return swarm.ZeroAddress, err
		}
	}
	err = putter.Done(encryptedReference)
	if err != nil {
		logger.Debug("done split encrypted reference failed", "error", err)
		logger.Error(nil, "done split encrypted reference failed")
		return swarm.ZeroAddress, err
	}

	w.Header().Set(SwarmActHistoryAddressHeader, historyReference.String())

	return encryptedReference, nil
}
