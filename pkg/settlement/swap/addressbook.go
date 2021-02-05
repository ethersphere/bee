// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	peerPrefix            = "swap_chequebook_peer_"
	peerChequebookPrefix  = "swap_peer_chequebook_"
	beneficiaryPeerPrefix = "swap_beneficiary_peer_"
	peerBeneficiaryPrefix = "swap_peer_beneficiary_"
)

// Addressbook maps peers to beneficaries, chequebooks and in reverse.
type Addressbook interface {
	// Beneficiary returns the beneficiary for the given peer.
	Beneficiary(peer swarm.Address) (beneficiary common.Address, known bool, err error)
	// Chequebook returns the chequebook for the given peer.
	Chequebook(peer swarm.Address) (chequebookAddress common.Address, known bool, err error)
	// BeneficiaryPeer returns the peer for a beneficiary.
	BeneficiaryPeer(beneficiary common.Address) (peer swarm.Address, known bool, err error)
	// ChequebookPeer returns the peer for a beneficiary.
	ChequebookPeer(chequebook common.Address) (peer swarm.Address, known bool, err error)
	// PutBeneficiary stores the beneficiary for the given peer.
	PutBeneficiary(peer swarm.Address, beneficiary common.Address) error
	// PutChequebook stores the chequebook for the given peer.
	PutChequebook(peer swarm.Address, chequebook common.Address) error
}

type addressbook struct {
	store storage.StateStorer
}

// NewAddressbook creates a new addressbook using the store.
func NewAddressbook(store storage.StateStorer) Addressbook {
	return &addressbook{
		store: store,
	}
}

// Beneficiary returns the beneficiary for the given peer.
func (a *addressbook) Beneficiary(peer swarm.Address) (beneficiary common.Address, known bool, err error) {
	err = a.store.Get(peerBeneficiaryKey(peer), &beneficiary)
	if err != nil {
		if err != storage.ErrNotFound {
			return common.Address{}, false, err
		}
		return common.Address{}, false, nil
	}
	return beneficiary, true, nil
}

// BeneficiaryPeer returns the peer for a beneficiary.
func (a *addressbook) BeneficiaryPeer(beneficiary common.Address) (peer swarm.Address, known bool, err error) {
	err = a.store.Get(beneficiaryPeerKey(beneficiary), &peer)
	if err != nil {
		if err != storage.ErrNotFound {
			return swarm.Address{}, false, err
		}
		return swarm.Address{}, false, nil
	}
	return peer, true, nil
}

// Chequebook returns the chequebook for the given peer.
func (a *addressbook) Chequebook(peer swarm.Address) (chequebookAddress common.Address, known bool, err error) {
	err = a.store.Get(peerKey(peer), &chequebookAddress)
	if err != nil {
		if err != storage.ErrNotFound {
			return common.Address{}, false, err
		}
		return common.Address{}, false, nil
	}
	return chequebookAddress, true, nil
}

// ChequebookPeer returns the peer for a beneficiary.
func (a *addressbook) ChequebookPeer(chequebook common.Address) (peer swarm.Address, known bool, err error) {
	err = a.store.Get(chequebookPeerKey(chequebook), &peer)
	if err != nil {
		if err != storage.ErrNotFound {
			return swarm.Address{}, false, err
		}
		return swarm.Address{}, false, nil
	}
	return peer, true, nil
}

// PutBeneficiary stores the beneficiary for the given peer.
func (a *addressbook) PutBeneficiary(peer swarm.Address, beneficiary common.Address) error {
	err := a.store.Put(peerBeneficiaryKey(peer), beneficiary)
	if err != nil {
		return err
	}
	return a.store.Put(beneficiaryPeerKey(beneficiary), peer)
}

// PutChequebook stores the chequebook for the given peer.
func (a *addressbook) PutChequebook(peer swarm.Address, chequebook common.Address) error {
	err := a.store.Put(peerKey(peer), chequebook)
	if err != nil {
		return err
	}
	return a.store.Put(chequebookPeerKey(chequebook), peer)
}

// peerKey computes the key where to store the chequebook from a peer.
func peerKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", peerPrefix, peer)
}

// chequebookPeerKey computes the key where to store the peer for a chequebook.
func chequebookPeerKey(chequebook common.Address) string {
	return fmt.Sprintf("%s%s", peerChequebookPrefix, chequebook)
}

// peerBeneficiaryKey computes the key where to store the beneficiary for a peer.
func peerBeneficiaryKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", peerBeneficiaryPrefix, peer)
}

// beneficiaryPeerKey computes the key where to store the peer for a beneficiary.
func beneficiaryPeerKey(peer common.Address) string {
	return fmt.Sprintf("%s%s", beneficiaryPeerPrefix, peer)
}
