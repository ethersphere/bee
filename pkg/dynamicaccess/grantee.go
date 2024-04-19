package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	publicKeyLen = 65
)

type GranteeList interface {
	Add(addList []*ecdsa.PublicKey) error
	Remove(removeList []*ecdsa.PublicKey) error
	Get() []*ecdsa.PublicKey
	Save(ctx context.Context) (swarm.Address, error)
}

type GranteeListStruct struct {
	grantees []byte
	loadSave file.LoadSaver
	putter   storer.PutterSession
}

var _ GranteeList = (*GranteeListStruct)(nil)

func (g *GranteeListStruct) Get() []*ecdsa.PublicKey {
	return g.deserialize(g.grantees)
}

func (g *GranteeListStruct) serialize(publicKeys []*ecdsa.PublicKey) []byte {
	b := make([]byte, 0, len(publicKeys)*publicKeyLen)
	for _, key := range publicKeys {
		b = append(b, g.serializePublicKey(key)...)
	}
	return b
}

func (g *GranteeListStruct) serializePublicKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)
}

func (g *GranteeListStruct) deserialize(data []byte) []*ecdsa.PublicKey {
	if len(data) == 0 {
		return nil
	}

	p := make([]*ecdsa.PublicKey, 0, len(data)/publicKeyLen)
	for i := 0; i < len(data); i += publicKeyLen {
		pubKey := g.deserializeBytes(data[i : i+publicKeyLen])
		if pubKey == nil {
			return nil
		}
		p = append(p, pubKey)
	}
	return p
}

func (g *GranteeListStruct) deserializeBytes(data []byte) *ecdsa.PublicKey {
	curve := elliptic.P256()
	x, y := elliptic.Unmarshal(curve, data)
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
}

func (g *GranteeListStruct) Add(addList []*ecdsa.PublicKey) error {
	if len(addList) == 0 {
		return fmt.Errorf("no public key provided")
	}

	data := g.serialize(addList)
	g.grantees = append(g.grantees, data...)
	return nil
}

func (g *GranteeListStruct) Save(ctx context.Context) (swarm.Address, error) {
	refBytes, err := g.loadSave.Save(ctx, g.grantees)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("grantee save error: %w", err)
	}
	address := swarm.NewAddress(refBytes)
	err = g.putter.Done(address)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return address, nil
}

func (g *GranteeListStruct) Remove(keysToRemove []*ecdsa.PublicKey) error {
	if len(keysToRemove) == 0 {
		return fmt.Errorf("nothing to remove")
	}
	grantees := g.deserialize(g.grantees)
	if grantees == nil {
		return fmt.Errorf("no grantee found")
	}

	for _, remove := range keysToRemove {
		for i, grantee := range grantees {
			if grantee.Equal(remove) {
				grantees[i] = grantees[len(grantees)-1]
				grantees = grantees[:len(grantees)-1]
			}
		}
	}
	g.grantees = g.serialize(grantees)
	return nil
}

func NewGranteeList(ls file.LoadSaver, putter storer.PutterSession, reference swarm.Address) GranteeList {
	var (
		data []byte
		err  error
	)
	if swarm.ZeroAddress.Equal(reference) || swarm.EmptyAddress.Equal(reference) {
		data = []byte{}
	} else {
		data, err = ls.Load(context.Background(), reference.Bytes())
	}
	if err != nil {
		return nil
	}

	return &GranteeListStruct{
		grantees: data,
		loadSave: ls,
		putter:   putter,
	}
}

func (g *GranteeListStruct) Store() (swarm.Address, error) {
	return swarm.EmptyAddress, nil
}
