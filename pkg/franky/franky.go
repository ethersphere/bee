package franky

import "errors"

const (
	shards        = 32
	shardCapacity = int64(1000)
)

var ErrNoSpace = errors.New("no space")

type Franky struct {
	shards []*shard
}

func New(basepath string) *Franky {
	f := &Franky{}

	// try to get the persisted offsets, check if dirty file exists
	for i := 0; i < shards; i++ {
		sh, err := newShard(i, shardCapacity, 4096, basepath, nil)
		if err != nil {
			panic("create shard")
		}
		f.shards = append(f.shards, sh)
	}
	return f
}

func (f *Franky) Write(data []byte) (shard uint8, o int64, fail func(), err error) {
	for shard = 0; shard < shards; shard++ {
		o, fail = f.shards[shard].freeOffset()
		if o > -1 {
			break
		}
	}

	if o == -1 {
		return 0, 0, nil, ErrNoSpace
	}

	if err = f.shards[shard].writeAt(data, o); err != nil {
		fail()
		return 0, 0, nil, err
	}
	return shard, o, fail, nil
}

func (f *Franky) Read(shard uint8, offset int64) ([]byte, error) {
	return f.shards[shard].readAt(offset)
}

func (f *Franky) Close() error {
	for _, s := range f.shards {
		_, _ = s.close()
	}
	return nil
}
