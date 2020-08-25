// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"
)

func wrapDial(ep string) (*ethclient.Client, error) {

	// Open a connection to the ethereum node through the endpoint.
	cl, err := ethclient.Dial(ep)
	if err != nil {
		return nil, err
	}

	// Ensure the ENS resolver contract is deployed on the network we are now
	// connected to.
	if _, err := goens.PublicResolverAddress(cl); err != nil {
		return nil, err
	}

	return cl, nil
}

func wrapResolve(backend bind.ContractBackend, name string) (string, error) {

	// Connect to the ENS resolver for the provided name.
	ensR, err := goens.NewResolver(backend, name)
	if err != nil {
		return "", err
	}

	// Try and read out the content hash record.
	ch, err := ensR.Contenthash()
	if err != nil {
		return "", err
	}

	adr, err := goens.ContenthashToString(ch)
	if err != nil {
		return "", err
	}

	return adr, nil
}
