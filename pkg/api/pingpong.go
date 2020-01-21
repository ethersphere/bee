package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

func (s *server) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimPrefix(r.URL.Path, "/pingpong")

	addr, err := multiaddr.NewMultiaddr(target)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "invalid address", target, err)
		return
	}

	ctx := r.Context()

	peerID, err := s.P2P.Connect(ctx, addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "connect error", addr, err)
		return
	}

	rtt, err := s.Pingpong.Ping(ctx, peerID, "hey", "there", ",", "how are", "you", "?")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "ping error", addr, err)
		return
	}

	fmt.Fprintln(w, "RTT", rtt)
}
