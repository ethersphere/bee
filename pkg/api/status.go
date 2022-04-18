package api

import (
	"net/http"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type statusResponse struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	APIVersion      string `json:"apiVersion"`
	DebugAPIVersion string `json:"debugApiVersion"`
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, statusResponse{
		Status:          "ok",
		Version:         bee.Version,
		APIVersion:      Version,
		DebugAPIVersion: Version,
	})
}
