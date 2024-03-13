package dynamicaccess

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
)

func setupAccessLogic() AccessLogic {
	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		errors.New("error creating private key")
	}
	al := NewAccessLogic(privateKey)

	return al
}

func TestGetLookupKey_Success(t *testing.T) {
	al := setupAccessLogic()

	publisher := "examplePublisher"
	tag := "exampleTag"

	lookupKey, err := al.GetLookUpKey(publisher, tag)
	if err != nil {
		t.Errorf("Could not fetch lookup key from publisher and tag")
	}

	expectedLookupKey := "expectedLookupKey"
	if lookupKey != expectedLookupKey {
		t.Errorf("The lookup key that was returned is not correct!")
	}
}

func TestGetLookupKey_Error(t *testing.T) {
	al := setupAccessLogic()

	invalidPublisher := ""
	tag := "exampleTag"

	lookupKey, err := al.GetLookUpKey(invalidPublisher, tag)
	if err != nil {
		t.Errorf("There was an error while fetching lookup key")
	}

	if lookupKey != "" {
		t.Errorf("Expected lookup key to be empty for invalid input")
	}
}

func TestGetAccessKeyDecriptionKey_Success(t *testing.T) {
	al := setupAccessLogic()

	publisher := "examplePublisher"
	tag := "exampleTag"

	access_key_decryption_key, err := al.GetAccessKeyDecriptionKey(publisher, tag)
	if err != nil {
		t.Errorf("GetAccessKeyDecriptionKey gave back error")
	}

	expectedResult := "we-dont-know"
	if access_key_decryption_key != expectedResult {
		t.Errorf("The access key decryption key is not correct!")
	}
}

func TestGetAccessKeyDecriptionKey_Error(t *testing.T) {
	al := setupAccessLogic()

	invalidPublisher := ""
	tag := "exampleTag"

	access_key_decryption_key, err := al.GetAccessKeyDecriptionKey(invalidPublisher, tag)
	if err != nil {
		t.Errorf("GetAccessKeyDecriptionKey gave back error")
	}

	if access_key_decryption_key != "" {
		t.Errorf("GetAccessKeyDecriptionKey should give back empty string for invalid input!")
	}
}

func TestGetEncryptedAccessKey_Success(t *testing.T) {
	al := setupAccessLogic()

	actRootHash := "0xabcexample"
	lookupKey := "exampleLookupKey"

	encrypted_access_key, err := al.GetEncryptedAccessKey(actRootHash, lookupKey)
	if err != nil {
		t.Errorf("There was an error while executing GetEncryptedAccessKey")
	}

	expectedEncryptedKey := "abc013encryptedkey"
	if encrypted_access_key != expectedEncryptedKey {
		t.Errorf("GetEncryptedAccessKey didn't give back the expected value!")
	}
}

func TestGetEncryptedAccessKey_Error(t *testing.T) {
	al := setupAccessLogic()

	actRootHash := "0xabcexample"
	lookupKey := "exampleLookupKey"

	empty_act_result, _ := al.GetEncryptedAccessKey("", lookupKey)
	if empty_act_result != nil {
		t.Errorf("GetEncryptedAccessKey should give back nil for empty act root hash!")
	}

	empty_lookup_result, _ := al.GetEncryptedAccessKey(actRootHash, "")

	if empty_lookup_result != nil {
		t.Errorf("GetEncryptedAccessKey should give back nil for empty lookup key!")
	}
}

func TestGet_Success(t *testing.T) {
	al := setupAccessLogic()

	actRootHash := "0xabcexample"
	encryptedRef := "bzzabcasab"
	publisher := "examplePublisher"
	tag := "exampleTag"

	ref, err := al.Get(actRootHash, encryptedRef, publisher, tag)
	if err != nil {
		t.Errorf("There was an error while calling Get")
	}

	expectedRef := "bzzNotEncrypted128long"
	if ref != expectedRef {
		t.Errorf("Get gave back wrong Swarm reference!")
	}
}

func TestGet_Error(t *testing.T) {
	al := setupAccessLogic()

	actRootHash := "0xabcexample"
	encryptedRef := "bzzabcasab"
	publisher := "examplePublisher"
	tag := "exampleTag"

	refOne, _ := al.Get("", encryptedRef, publisher, tag)
	if refOne != "" {
		t.Errorf("Get should give back empty string if ACT root hash not provided!")
	}

	refTwo, _ := al.Get(actRootHash, "", publisher, tag)
	if refTwo != "" {
		t.Errorf("Get should give back empty string if encrypted ref not provided!")
	}

	refThree, _ := al.Get(actRootHash, encryptedRef, "", tag)
	if refThree != "" {
		t.Errorf("Get should give back empty string if publisher not provided!")
	}

	refFour, _ := al.Get(actRootHash, encryptedRef, publisher, "")
	if refFour != "" {
		t.Errorf("Get should give back empty string if tag was not provided!")
	}
}

func TestNewAccessLogic(t *testing.T) {
	logic := setupAccessLogic()

	_, ok := logic.(*DefaultAccessLogic)
	if !ok {
		t.Errorf("NewAccessLogic: expected type *DefaultAccessLogic, got %T", logic)
	}
}
