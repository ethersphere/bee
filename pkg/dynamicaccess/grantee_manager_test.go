package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
)

func setupAccessLogic(privateKey *ecdsa.PrivateKey) dynamicaccess.AccessLogic {
	// privateKey, err := crypto.GenerateSecp256k1Key()
	// if err != nil {
	// 	errors.New("error creating private key")
	// }
	si := dynamicaccess.NewDefaultSession(privateKey)
	al := dynamicaccess.NewAccessLogic(si)

	return al
}

func TestAdd(t *testing.T) {
	act := dynamicaccess.NewInMemoryAct()
	m := dynamicaccess.NewGranteeManager(setupAccessLogic(getPrivateKey()))
	pub, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	id1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	err := m.Add("topic", []*ecdsa.PublicKey{&id1.PublicKey})
	if err != nil {
		t.Errorf("Add() returned an error")
	}
	err = m.Add("topic", []*ecdsa.PublicKey{&id2.PublicKey})
	if err != nil {
		t.Errorf("Add() returned an error")
	}
	m.Publish(act, pub.PublicKey, "topic")
	fmt.Println("")
}
