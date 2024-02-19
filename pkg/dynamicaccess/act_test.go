package dynamicaccess

import "testing"

func TestAdd(t *testing.T) {
	a := NewAct()
	_, err := a.Add("", "")
	if err != nil {
		t.Error("Add() should not return an error")
	}
}

func TestGet(t *testing.T) {
	a := NewAct()
	_, err := a.Get("")
	if err != nil {
		t.Error("Get() should not return an error")
	}
}
