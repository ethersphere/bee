package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *server) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	peerID := mux.Vars(r)["peer-id"]
	ctx := r.Context()

	rtt, err := s.Pingpong.Ping(ctx, peerID, "hey", "there", ",", "how are", "you", "?")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "ping error", peerID, err)
		return
	}

	fmt.Fprintln(w, "RTT", rtt)
}
