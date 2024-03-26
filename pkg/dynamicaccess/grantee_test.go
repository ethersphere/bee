package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
)

var _ dynamicaccess.GranteeList = (*dynamicaccess.GranteeListStruct)(nil)

func TestGranteeAddGrantees(t *testing.T) {
	grantee := dynamicaccess.NewGrantee()

	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	exampleTopic := "topic"
	err = grantee.Add(exampleTopic, addList)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	grantees := grantee.Get(exampleTopic)
	if !reflect.DeepEqual(grantees, addList) {
		t.Errorf("Expected grantees %v, got %v", addList, grantees)
	}
}

func TestRemoveGrantees(t *testing.T) {
	grantee := dynamicaccess.NewGrantee()

	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	exampleTopic := "topic"
	err = grantee.Add(exampleTopic, addList)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	removeList := []*ecdsa.PublicKey{&key1.PublicKey}
	err = grantee.Remove(exampleTopic, removeList)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	grantees := grantee.Get(exampleTopic)
	expectedGrantees := []*ecdsa.PublicKey{&key2.PublicKey}

	for i, grantee := range grantees {
		if grantee != expectedGrantees[i] {
			t.Errorf("Expected grantee %v, got %v", expectedGrantees[i], grantee)
		}
	}
}

func TestGetGrantees(t *testing.T) {
	grantee := dynamicaccess.NewGrantee()

	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	exampleTopic := "topic"
	err = grantee.Add(exampleTopic, addList)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	grantees := grantee.Get(exampleTopic)
	for i, grantee := range grantees {
		if grantee != addList[i] {
			t.Errorf("Expected grantee %v, got %v", addList[i], grantee)
		}
	}
}
