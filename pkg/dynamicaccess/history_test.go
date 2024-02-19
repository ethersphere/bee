package dynamicaccess

import "testing"

func TestHistoryAdd(t *testing.T) {
	newRootHash, err := NewHistory().Add("", "")
	if err != nil {
		t.Errorf("Error adding history: %v", err)
	}
	_ = newRootHash // Ignore the newRootHash if not needed
}

func TestHistoryGet(t *testing.T) {
	value, err := NewHistory().Get("")
	if err != nil {
		t.Errorf("Error getting history: %v", err)
	}
	_ = value // Ignore the value if not needed
}
