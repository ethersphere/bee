package dynamicaccess

import (
	"crypto/ecdsa"
)

type Grantee interface {
	AddGrantees(topic string, addList []*ecdsa.PublicKey) error
	RemoveGrantees(topic string, removeList []*ecdsa.PublicKey) error
	GetGrantees(topic string) []*ecdsa.PublicKey
}

type defaultGrantee struct {
	grantees map[string][]*ecdsa.PublicKey
}

func (g *defaultGrantee) GetGrantees(topic string) []*ecdsa.PublicKey {
	grantees := g.grantees[topic]
	keys := make([]*ecdsa.PublicKey, len(grantees))
	copy(keys, grantees)
	return keys
}

func (g *defaultGrantee) AddGrantees(topic string, addList []*ecdsa.PublicKey) error {
	g.grantees[topic] = append(g.grantees[topic], addList...)
	return nil
}

func (g *defaultGrantee) RemoveGrantees(topic string, removeList []*ecdsa.PublicKey) error {
	for _, remove := range removeList {
		for i, grantee := range g.grantees[topic] {
			if *grantee == *remove {
				g.grantees[topic][i] = g.grantees[topic][len(g.grantees[topic])-1]
				g.grantees[topic] = g.grantees[topic][:len(g.grantees[topic])-1]
			}
		}
	}

	
	return nil
}

func NewGrantee() Grantee {
	return &defaultGrantee{grantees: make(map[string][]*ecdsa.PublicKey)}
}