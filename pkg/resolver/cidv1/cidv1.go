package cidv1

import (
	"fmt"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// https://github.com/multiformats/multicodec/blob/master/table.csv
const (
	SwarmNsCodec       uint64 = 0xe4
	SwarmManifestCodec uint64 = 0xfa
	SwarmFeedCodec     uint64 = 0xfb
)

type Resolver struct{}

func (Resolver) Resolve(name string) (swarm.Address, error) {
	id, err := cid.Parse(name)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("failed parsing CID %s err %w", name, err)
	}

	switch id.Prefix().GetCodec() {
	case SwarmNsCodec:
	case SwarmManifestCodec:
	case SwarmFeedCodec:
	default:
		return swarm.ZeroAddress, fmt.Errorf("unsupported codec for CID %d", id.Prefix().GetCodec())
	}

	dh, err := multihash.Decode(id.Hash())
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("unable to decode hash %w", err)
	}

	return swarm.NewAddress(dh.Digest), nil
}

func (Resolver) Close() error {
	return nil
}
