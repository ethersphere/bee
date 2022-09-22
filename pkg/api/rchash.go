package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/gorilla/mux"
)

type rchash struct {
	Sample storage.Sample
	Time   string
}

func (s *Service) rchasher(w http.ResponseWriter, r *http.Request) {

	start := time.Now()
	depthStr := mux.Vars(r)["depth"]

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Error(err, "reserve commitment hasher: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	if depth > 255 {
		depth = 255
	}

	anchorStr := mux.Vars(r)["anchor"]
	anchor := []byte(anchorStr)

	sample, err := s.storer.ReserveSample(r.Context(), anchor, uint8(depth))
	if err != nil {
		jsonhttp.BadRequest(w, "invalid")
		return
	}

	jsonhttp.OK(w, rchash{
		Sample: sample,
		Time:   time.Since(start).String(),
	})
}
