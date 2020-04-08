package persistent

import (
	"fmt"
	"strings"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

const keyPrefix = "addressbook_entry_"

type store struct {
	store storage.StateStorer
}

func New(storer storage.StateStorer) addressbook.GetPutter {
	return &store{
		store: storer,
	}
}

func (s *store) Get(overlay swarm.Address) (addr ma.Multiaddr, err error) {
	key := keyPrefix + overlay.String()
	v := addressbook.PeerEntry{}

	err = s.store.Get(key, &v)
	if err != nil {
		return nil, err
	}
	return v.Multiaddr, nil
}

func (s *store) Put(overlay swarm.Address, addr ma.Multiaddr) (exists bool, err error) {
	if _, err = s.Get(overlay); err == nil {
		return true, nil
	}

	key := keyPrefix + overlay.String()
	pe := &addressbook.PeerEntry{Overlay: overlay, Multiaddr: addr}

	err = s.store.Put(key, pe)
	return false, err
}

func (s *store) Overlays() (overlays []swarm.Address) {
	err := s.store.Iterate(keyPrefix, func(key, value []byte) (stop bool, err error) {
		k := string(key)
		if !strings.HasPrefix(k, keyPrefix) {
			return true, nil
		}
		split := strings.SplitAfter(k, keyPrefix)
		if len(split) != 2 {
			return fmt.Errorf("invalid overlay key: %s", k)
		}
		addr, err := swarm.ParseHexAddress(split[1])
		if err != nil {
			return true, err
		}
		overlays = append(overlays, addr)
	})

	return overlays
}

func (s *store) Multiaddresses() (multis []ma.Multiaddr) {
	err := s.store.Iterate(keyPrefix, func(key, value []byte) (stop bool, err error) {
		v := string(value)
	})

	return []ma.Multiaddr{}
}
