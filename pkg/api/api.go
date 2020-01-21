package api

import (
	"net/http"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/pingpong"
)

type server struct {
	Options
	http.Handler
}

type Options struct {
	P2P      p2p.Service
	Pingpong *pingpong.Service
}

func New(o Options) http.Handler {
	s := &server{
		Options: o,
	}

	s.setupRouting()

	return s
}
