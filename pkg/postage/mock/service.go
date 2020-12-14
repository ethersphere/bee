package mock

import "github.com/ethersphere/bee/pkg/postage"

func WithMockBatch(id []byte) Option {
	return optionFunc(func(m *mockPostage) {

	})
}

type Option interface {
	apply(*mockPostage)
}
type optionFunc func(*mockPostage)

func (f optionFunc) apply(r *mockPostage) { f(r) }

func New(o ...Option) postage.Service {
	m := &mockPostage{}
	for _, v := range o {
		v.apply(m)
	}

	// add the fallback value we have in the api right now.
	// in the future this needs to go away once batch id becomes
	// de facto mandatory in the package

	id := make([]byte, 32)
	st := postage.NewStampIssuer("test fallback", "test identity", id, 24, 6)
	m.Add(st)

	return m
}

type mockPostage struct {
	i *postage.StampIssuer
}

func (m *mockPostage) Add(s *postage.StampIssuer) {
	m.i = s
}

func (m *mockPostage) StampIssuers() []*postage.StampIssuer {
	panic("not implemented") // TODO: Implement
}

func (m *mockPostage) GetStampIssuer(_ []byte) (*postage.StampIssuer, error) {
	return m.i, nil
}

func (m *mockPostage) Load() error {
	panic("not implemented") // TODO: Implement
}

func (m *mockPostage) Save() error {
	panic("not implemented") // TODO: Implement
}
