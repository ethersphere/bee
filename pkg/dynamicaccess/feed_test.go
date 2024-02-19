package dynamicaccess

import "testing"

func TestFeedUpdate(t *testing.T) {
	err := NewFeed().Update("", "")
	if err != nil {
		t.Errorf("Error updating feed: %v", err)
	}
}

func TestFeedGet(t *testing.T) {
	content, err := NewFeed().Get("")
	if err != nil {
		t.Errorf("Error getting feed: %v", err)
	}
	_ = content // Ignore the content if not needed
}

func TestFeedAddNewGrantee(t *testing.T) {
	err := NewFeed().AddNewGrantee("", "")
	if err != nil {
		t.Errorf("Error adding new grantee to feed: %v", err)
	}
}

func TestFeedRemoveGrantee(t *testing.T) {
	err := NewFeed().RemoveGrantee("", "")
	if err != nil {
		t.Errorf("Error removing grantee from feed: %v", err)
	}
}

func TestFeedGetAccess(t *testing.T) {
	access, err := NewFeed().GetAccess("", "", "")
	if err != nil {
		t.Errorf("Error getting access to feed: %v", err)
	}
	_ = access // Ignore the access if not needed
}
