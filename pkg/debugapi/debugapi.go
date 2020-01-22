package debugapi

import (
	"net/http"
)

type server struct {
	Options
	http.Handler
}

type Options struct{}

func New(o Options) http.Handler {
	s := &server{
		Options: o,
	}

	s.setupRouting()

	return s
}
