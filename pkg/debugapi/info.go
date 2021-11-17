package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type BeeNodeMode uint

const (
	LightMode BeeNodeMode = iota
	FullMode
	DevMode
)

type infoResponse struct {
	BeeMode     string `json:"beeMode"`
	GatewayMode bool   `json:"gatewayMode"`
}

func (b BeeNodeMode) String() string {
	switch b {
	case LightMode:
		return "light"
	case FullMode:
		return "full"
	case DevMode:
		return "dev"
	}
	return "unknown"
}

// infoGetHandler gives back information about the Bee node configuration.
func (s *Service) infoGetHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, infoResponse{
		BeeMode:     s.beeMode.String(),
		GatewayMode: s.gatewayMode,
	})
}
