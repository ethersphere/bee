package api

import "net/http"

func (s *Service) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if s.probe != nil && s.probe.Ready() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
}
