package mock

type subscriber struct {
	stable bool
}

func NewSubscriber(stable bool) *subscriber {
	return &subscriber{
		stable: stable,
	}
}

func (s *subscriber) Subscribe() <-chan struct{} {
	c := make(chan struct{})
	if s.stable {
		close(c)
	}
	return c
}

func (s *subscriber) IsStabilized() bool {
	return s.stable
}
