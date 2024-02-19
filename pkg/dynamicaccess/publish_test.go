package dynamicaccess

import "testing"

func TestUpload(t *testing.T) {
	p := &DefaultPublish{}
	_, err := p.upload("test")
	if err != nil {
		t.Errorf("Error uploading file: %v", err)
	}
}
