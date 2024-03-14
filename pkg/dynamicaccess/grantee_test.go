package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
)

// func TestGranteeRevoke(t *testing.T) {
// 	err := NewGrantee().Revoke("")
// 	if err != nil {
// 		t.Errorf("Error revoking grantee: %v", err)
// 	}
// }

/*func TestGranteeRevokeList(t *testing.T) {
	_, err := NewGrantee().RevokeList("", nil, nil)
	if err != nil {
		t.Errorf("Error revoking list of grantees: %v", err)
	}
}*/

// func TestGranteePublish(t *testing.T) {
// 	err := NewGrantee().Publish("")
// 	if err != nil {
// 		t.Errorf("Error publishing grantee: %v", err)
// 	}
// }

func TestGranteeAddGrantees(t *testing.T) {
	// Create a new Grantee
	grantee := dynamicaccess.NewGrantee()

	// Generate some dummy ecdsa.PublicKey values
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Add the keys to the grantee
	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	err := grantee.AddGrantees("topicName", addList)
	grantees := grantee.GetGrantees("topicName")
	// Check for errors
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if the keys were added correctly
	if !reflect.DeepEqual(grantees, addList) {
		t.Errorf("Expected grantees %v, got %v", addList, grantees)
	}
}

func TestRemoveGrantees(t *testing.T) {
	// Create a new Grantee
	grantee := dynamicaccess.NewGrantee()

	// Generate some dummy ecdsa.PublicKey values
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Add the keys to the grantee
	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	grantee.AddGrantees("topicName", addList)

	// Remove one of the keys
	removeList := []*ecdsa.PublicKey{&key1.PublicKey}
	err := grantee.RemoveGrantees("topicName", removeList)
	grantees := grantee.GetGrantees("topicName")

	// Check for errors
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if the key was removed correctly
	expectedGrantees := []*ecdsa.PublicKey{&key2.PublicKey}
	if !reflect.DeepEqual(grantees, expectedGrantees) {
		t.Errorf("Expected grantees %v, got %v", expectedGrantees, grantees)
	}
}

func TestGetGrantees(t *testing.T) {
	// Create a new Grantee
	grantee := dynamicaccess.NewGrantee()

	// Generate some dummy ecdsa.PublicKey values
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Add the keys to the grantee
	addList := []*ecdsa.PublicKey{&key1.PublicKey, &key2.PublicKey}
	grantee.AddGrantees("topicName", addList)

	// Get the grantees
	grantees := grantee.GetGrantees("topicName")

	// Check if the grantees were returned correctly
	if !reflect.DeepEqual(grantees, addList) {
		t.Errorf("Expected grantees %v, got %v", addList, grantees)
	}
}
