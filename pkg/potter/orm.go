package potter

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/potter/pot"
)

// Feature models an attribute and defines its serialisation
type Feature struct {
	Name   string // mnemonic reference to attribute column
	Size   int    // encoding length
	Encode func(Model, []byte) error
	Decode func(Model, []byte) error
}

// Model is the generic type for mappable object that has features
type Model interface {
	fmt.Stringer
}

// Record encapsulates a model object with its explicitly defined features remembered
type Record struct {
	Model
	features []string
}

// NewRecord creates a record out of a Model object and a list of already specified features
func NewRecord(m Model, fs ...string) *Record {
	return &Record{m, fs}
}

func (r *Record) Set(fs ...string) {
	r.features = append(r.features, fs...)
}

func (r *Record) Unset(fs ...string) {
	for _, fe := range fs {
		for i, fee := range r.features {
			if fee == fe {
				r.features = append(r.features[:i], r.features[i+1:]...)
				break
			}
		}
	}
}

// Schema is a set of features
type Schema struct {
	Features []Feature
}

// NewSchema is the constructor for a schema
func NewSchema(fs []Feature) *Schema {
	return &Schema{Features: append([]Feature{}, fs...)}
}

// Get finds a feature of the schema by name
func (s *Schema) Get(name string) (f Feature, err error) {
	for _, fe := range s.Features {
		if name == fe.Name {
			return fe, nil
		}
	}
	return f, ErrFeatureNotFound
}

// Slice returns a subschema of a schema from a subset of features of the receiver schema
func (s *Schema) Slice(fs []string) (*Schema, error) {
	fes := make([]Feature, len(fs))
	for i, f := range fs {
		fe, err := s.Get(f)
		if err != nil {
			return nil, err
		}
		fes[i] = fe
	}
	return &Schema{fes}, nil
}

// Facet is a description of an indexing on a schema
type Facet struct {
	Name string
	Key  []string
	Val  []string
}

// FORM (Faceted Object-Relational Mapping) represents an index on the pottery Models
// based on a facet specification
type FORM struct {
	Facet
	key  *Schema  // the key schema
	val  *Schema  // the value schema
	pot  *Index   // the actual index implemented by a persisted pot
	mode pot.Mode // the pot mode
}

// Entry implements pot Entry
var _ pot.Entry = (*Entry)(nil)

type Entry struct {
	key []byte
	val []byte
	// rec Record
}

func (e *Entry) Key() []byte {
	return e.key
}

func (e *Entry) String() string {
	return fmt.Sprintf("key: %x; val; %v", e.key, e.val)
}

func (e *Entry) Equal(v pot.Entry) bool {
	ev, ok := v.(*Entry)
	if !ok {
		return false
	}
	return bytes.Equal(e.val, ev.val)
}

func (e *Entry) MarshalBinary() ([]byte, error) {
	return e.val, nil

}
func (e *Entry) UnmarshalBinary(v []byte) error {
	e.val = v
	return nil
}

// NewEntry on a FORM returns an Entry
func (f *FORM) NewEntry(r *Record) (*Entry, error) {
	keyBuf := make([]byte, f.key.Size())
	if err := f.key.Encode(r, keyBuf); err != nil {
		return nil, err
	}
	valBuf := make([]byte, f.val.Size())
	if err := f.val.Encode(r, valBuf); err != nil {
		return nil, err
	}
	return &Entry{
		key: keyBuf,
		val: valBuf,
	}, nil
}

// Find constructs an entry from the specified fields of the record
// and retrieves the unique entry matching the key from the form pot or returns NotFound
func (f *FORM) Find(ctx context.Context, r *Record) error {
	e, err := f.NewEntry(r)
	if err != nil {
		return err
	}
	result, err := f.pot.Find(ctx, e.key)
	if err != nil {
		return err
	}
	return f.val.Decode(r, result.(*Entry).val)
}

// Add constructs an entry from the specified fields of the record
// and inserts the unique entry with the key and value based on the facet
func (f *FORM) Add(ctx context.Context, r *Record) error {
	e, err := f.NewEntry(r)
	if err != nil {
		return err
	}
	return f.pot.Add(ctx, e)
}

// Delete removes an entry constructed with the facet from the index
func (f *FORM) Delete(ctx context.Context, r *Record) error {
	e, err := f.NewEntry(r)
	if err != nil {
		return err
	}
	return f.pot.Delete(ctx, e.key)
}

// Encode serialises the set of features represented by the schema
// by encoding the respective features of the input model record
func (f *Schema) Encode(r *Record, b []byte) error {
	var from, to int
	for _, fe := range f.Features {
		to = from + fe.Size
		if err := fe.Encode(r.Model, b[from:to]); err != nil {
			return err
		}
		from = to
	}
	return nil
}

// Decode deserialises the input bytes based on the set of features represented by the schema
// and enriches the model record with the respective decoded feature values
func (f *Schema) Decode(r *Record, b []byte) error {
	var from, to int
OUTER:
	for _, fe := range f.Features {
		to = from + fe.Size
		for _, f := range r.features {
			if fe.Name == f {
				// record has got the feature specified already
				from = to
				continue OUTER
			}
		}
		if err := fe.Decode(r.Model, b[from:to]); err != nil {
			return err
		}
		r.features = append(r.features, fe.Name)
		from = to
	}
	return nil
}

// Size return the encoding size of the features of the schema
func (f *Schema) Size() (s int) {
	for _, fe := range f.Features {
		s += fe.Size
	}
	return s
}
