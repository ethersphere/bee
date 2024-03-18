// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

type BeeNodeMode uint

const (
	UnknownMode BeeNodeMode = iota
	LightMode
	FullMode
	DevMode
	UltraLightMode
)

type nodeResponse struct {
	BeeMode           string `json:"beeMode"`
	ChequebookEnabled bool   `json:"chequebookEnabled"`
	SwapEnabled       bool   `json:"swapEnabled"`
}

func (b BeeNodeMode) String() string {
	switch b {
	case LightMode:
		return "light"
	case FullMode:
		return "full"
	case DevMode:
		return "dev"
	case UltraLightMode:
		return "ultra-light"
	}
	return "unknown"
}

// nodeGetHandler gives back information about the Bee node configuration.
func (s *Service) nodeGetHandler(w http.ResponseWriter, _ *http.Request) {
	jsonhttp.OK(w, nodeResponse{
		BeeMode:           s.beeMode.String(),
		ChequebookEnabled: s.chequebookEnabled,
		SwapEnabled:       s.swapEnabled,
	})
}
