package feeds

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

type id struct {
	topic []byte
	index []byte
}

func (i *id) MarshalBinary() ([]byte, error) {
	return crypto.LegacyKeccak256(append(i.topic, i.index...))
}

// Feed is representing an epoch based feed
type Feed struct {
	Topic []byte
	Owner common.Address
}

// New constructs an epoch based feed from a human readable topic and an ether address
func New(topic string, owner common.Address) (*Feed, error) {
	th, err := crypto.LegacyKeccak256([]byte(topic))
	if err != nil {
		return nil, err
	}
	return &Feed{th, owner}, nil
}

// update represents an update instance of a feed, i.e., pairing of a Feed with an Epoch
type update struct {
	*Feed
	*epoch
}

// id calculates the identifier if a  feed update to be used in single owner chunks
func (u *update) id() ([]byte, error) {
	index, err := u.epoch.MarshalBinary()
	if err != nil {
		return nil, err
	}
	i := &id{u.Topic, index}
	return i.MarshalBinary()
}

// address calculates the soc address of a feed update
func (u *update) address() (addr swarm.Address, err error) {
	var i []byte
	i, err = u.id()
	if err != nil {
		return addr, err
	}
	owner, err := soc.NewOwner(u.Owner[:])
	if err != nil {
		return addr, err
	}
	return soc.CreateAddress(i, owner)
}
