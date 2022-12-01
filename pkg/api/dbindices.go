package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type StorageIndexDebugger interface {
	DebugIndices() (map[string]int, error)
}

func (s *Service) dbIndicesHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("db_indices").Build()

	indexDebugger, ok := s.storer.(StorageIndexDebugger)
	if !ok {
		jsonhttp.NotImplemented(w, "storage indices not available")
		logger.Error(nil, "db indices not implemented")
		return
	}

	indices, err := indexDebugger.DebugIndices()
	if err != nil {
		jsonhttp.InternalServerError(w, "cannot get storage indices")
		logger.Debug("db indices failed", "error", err)
		logger.Error(nil, "db indices failed")
		return
	}

	jsonhttp.OK(w, indices)
}
