package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	publicKeyLen = 65
)

// GranteeList manages a list of public keys.
type GranteeList interface {
	// Add adds a list of public keys to the grantee list. It filters out duplicates.
	Add(addList []*ecdsa.PublicKey) error
	// Remove removes a list of public keys from the grantee list, if there is any.
	Remove(removeList []*ecdsa.PublicKey) error
	// Get simply returns the list of public keys.
	Get() []*ecdsa.PublicKey
	// Save saves the grantee list to the underlying storage and returns the reference.
	Save(ctx context.Context) (swarm.Address, error)
}

type GranteeListStruct struct {
	grantees []*ecdsa.PublicKey
	loadSave file.LoadSaver
}

var _ GranteeList = (*GranteeListStruct)(nil)

func (g *GranteeListStruct) Get() []*ecdsa.PublicKey {
	return g.grantees
}

func (g *GranteeListStruct) Add(addList []*ecdsa.PublicKey) error {
	if len(addList) == 0 {
		return fmt.Errorf("no public key provided")
	}
	filteredList := make([]*ecdsa.PublicKey, 0, len(addList))
	for _, addkey := range addList {
		add := true
		for _, granteekey := range g.grantees {
			if granteekey.Equal(addkey) {
				add = false
				break
			}
		}
		for _, filteredkey := range filteredList {
			if filteredkey.Equal(addkey) {
				add = false
				break
			}
		}
		if add {
			filteredList = append(filteredList, addkey)
		}
	}
	g.grantees = append(g.grantees, filteredList...)

	return nil
}

func (g *GranteeListStruct) Save(ctx context.Context) (swarm.Address, error) {
	data := serialize(g.grantees)
	refBytes, err := g.loadSave.Save(ctx, data)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("grantee save error: %w", err)
	}

	return swarm.NewAddress(refBytes), nil
}

func (g *GranteeListStruct) Remove(keysToRemove []*ecdsa.PublicKey) error {
	if len(keysToRemove) == 0 {
		return fmt.Errorf("nothing to remove")
	}

	if len(g.grantees) == 0 {
		return fmt.Errorf("no grantee found")
	}
	grantees := g.grantees

	for _, remove := range keysToRemove {
		for i := 0; i < len(grantees); i++ {
			if grantees[i].Equal(remove) {
				grantees[i] = grantees[len(grantees)-1]
				grantees = grantees[:len(grantees)-1]
			}
		}
	}
	g.grantees = grantees

	return nil
}

func NewGranteeList(ls file.LoadSaver) (GranteeList, error) {
	return &GranteeListStruct{
		grantees: []*ecdsa.PublicKey{},
		loadSave: ls,
	}, nil
}

func NewGranteeListReference(ls file.LoadSaver, reference swarm.Address) (GranteeList, error) {
	data, err := ls.Load(context.Background(), reference.Bytes())
	if err != nil {
		return nil, err
	}
	grantees := deserialize(data)

	return &GranteeListStruct{
		grantees: grantees,
		loadSave: ls,
	}, nil
}

func serialize(publicKeys []*ecdsa.PublicKey) []byte {
	b := make([]byte, 0, len(publicKeys)*publicKeyLen)
	for _, key := range publicKeys {
		b = append(b, serializePublicKey(key)...)
	}
	return b
}

func serializePublicKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)
}

func deserialize(data []byte) []*ecdsa.PublicKey {
	if len(data) == 0 {
		return []*ecdsa.PublicKey{}
	}

	p := make([]*ecdsa.PublicKey, 0, len(data)/publicKeyLen)
	for i := 0; i < len(data); i += publicKeyLen {
		pubKey := deserializeBytes(data[i : i+publicKeyLen])
		if pubKey == nil {
			return []*ecdsa.PublicKey{}
		}
		p = append(p, pubKey)
	}
	return p
}

func deserializeBytes(data []byte) *ecdsa.PublicKey {
	key, err := btcec.ParsePubKey(data)
	if err != nil {
		return nil
	}
	return key.ToECDSA()
}
