package debugapi

import (
	"fmt"
	"net/http"
)

func (s *server) statusHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `{"status":"ok"}`)
}
