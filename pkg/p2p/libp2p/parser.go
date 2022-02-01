package libp2p

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

type MockParser struct {
	networkID uint64
}

var ErrInvalidAddress = errors.New("invalid address")

func generateSignData(underlay, overlay []byte, networkID uint64) []byte {
	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID)
	signData := append([]byte("bee-handshake-"), underlay...)
	signData = append(signData, overlay...)
	return append(signData, networkIDBytes...)
}

func (m *MockParser) ParseAck(ack *pb.Ack, _ []byte) (*bzz.Address, error) {
	underlay, overlay, signature := ack.Address.Underlay, ack.Address.Overlay, ack.Address.Signature
	recoveredPK, err := crypto.Recover(signature, generateSignData(underlay, overlay, m.networkID))
	if err != nil {
		return nil, ErrInvalidAddress
	}

	multiUnderlay, err := ma.NewMultiaddrBytes(underlay)
	if err != nil {
		return nil, ErrInvalidAddress
	}

	ethAddress, err := crypto.NewEthereumAddress(*recoveredPK)
	if err != nil {
		return nil, fmt.Errorf("extract ethereum address: %v: %w", err, ErrInvalidAddress)
	}

	return &bzz.Address{
		Underlay:        multiUnderlay,
		Overlay:         swarm.NewAddress(overlay),
		Signature:       signature,
		Transaction:     ack.Transaction,
		EthereumAddress: ethAddress,
	}, nil
}
