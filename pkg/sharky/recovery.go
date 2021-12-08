package sharky

type Recovery struct {
	shards []*slots
}

func NewRecovery(s *Shards) *Recovery {
	var ss []*slots
	for _, sh := range s.shards {
		sl := sh.slots
		for i := range sl.data {
			sl.data[i] = 0x0
		}
		ss = append(ss, sl)
	}
	r := &Recovery{shards: ss}
	return r
}

func (r *Recovery) Add(loc Location) error {
	return nil
}

func (r *Recovery) Save() error {
	return nil
}
