package dynamicaccess

import (
	"crypto/ecdsa"
)

type Grantee interface {
	//? ÁTBESZÉLNI
	// Revoke(topic string) error
	// Publish(topic string) error

	// RevokeList(topic string, removeList []string, addList []string) (string, error)
	// RevokeGrantees(topic string, removeList []string) (string, error)
	AddGrantees(topic string, addList []*ecdsa.PublicKey) error
	RemoveGrantees(topic string, removeList []*ecdsa.PublicKey) error
	GetGrantees(topic string) []*ecdsa.PublicKey
}

type defaultGrantee struct {
	grantees map[string][]*ecdsa.PublicKey // Modified field name to start with an uppercase letter
}

func (g *defaultGrantee) GetGrantees(topic string) []*ecdsa.PublicKey {
	return g.grantees[topic]
}

// func (g *defaultGrantee) Revoke(topic string) error {
// 	return nil
// }

// func (g *defaultGrantee) RevokeList(topic string, removeList []string, addList []string) (string, error) {
// 	return "", nil
// }

// func (g *defaultGrantee) Publish(topic string) error {
// 	return nil
// }

func (g *defaultGrantee) AddGrantees(topic string, addList []*ecdsa.PublicKey) error {
	g.grantees[topic] = append(g.grantees[topic], addList...)
	return nil
}

func (g *defaultGrantee) RemoveGrantees(topic string, removeList []*ecdsa.PublicKey) error {
	for _, remove := range removeList {
		for i, grantee := range g.grantees[topic] {
			if grantee == remove {
				g.grantees[topic] = append(g.grantees[topic][:i], g.grantees[topic][i+1:]...)
			}
		}
	}
	return nil
}

func NewGrantee() Grantee {
	return &defaultGrantee{grantees: make(map[string][]*ecdsa.PublicKey)}
}
