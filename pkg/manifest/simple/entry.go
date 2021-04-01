// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package simple

// Entry is a representation of a single manifest entry.
type Entry interface {
	// Reference returns the address of the file in the entry.
	Reference() string
	// Metadata returns the metadata for this entry.
	Metadata() map[string]string
}

// entry is a JSON representation of a single manifest entry.
type entry struct {
	Ref  string            `json:"reference"`
	Meta map[string]string `json:"metadata,omitempty"`
}

// newEntry creates a new Entry struct and returns it.
func newEntry(reference string, metadata map[string]string) *entry {
	return &entry{
		Ref:  reference,
		Meta: metadata,
	}
}

func (me *entry) Reference() string {
	return me.Ref
}

func (me *entry) Metadata() map[string]string {
	return me.Meta
}
