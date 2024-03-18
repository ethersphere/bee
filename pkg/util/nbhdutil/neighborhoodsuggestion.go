// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nbhdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type httpClient interface {
	Get(url string) (*http.Response, error)
}

func FetchNeighborhood(client httpClient, suggester string) (string, error) {
	if suggester == "" {
		return "", nil
	}

	_, err := url.ParseRequestURI(suggester)
	if err != nil {
		return "", err
	}

	type suggestionRes struct {
		Neighborhood string `json:"neighborhood"`
	}
	res, err := client.Get(suggester)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var suggestion suggestionRes
	d, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(d, &suggestion)
	if err != nil {
		return "", err
	}
	_, err = swarm.ParseBitStrAddress(suggestion.Neighborhood)
	if err != nil {
		return "", fmt.Errorf("invalid neighborhood. %s", suggestion.Neighborhood)
	}
	return suggestion.Neighborhood, nil
}
