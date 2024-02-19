package dynamicaccess

import "testing"

func TestGranteeRevoke(t *testing.T) {
	err := NewGrantee().Revoke("")
	if err != nil {
		t.Errorf("Error revoking grantee: %v", err)
	}
}

func TestGranteeRevokeList(t *testing.T) {
	_, err := NewGrantee().RevokeList("", nil, nil)
	if err != nil {
		t.Errorf("Error revoking list of grantees: %v", err)
	}
}

func TestGranteePublish(t *testing.T) {
	err := NewGrantee().Publish("")
	if err != nil {
		t.Errorf("Error publishing grantee: %v", err)
	}
}
