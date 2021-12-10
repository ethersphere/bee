package sharky

import (
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/go-multierror"
)

type Recovery struct {
	shards []*slots
	files  []os.File
}

func NewRecovery(dir string, shardCnt int, shardSize uint32, datasize int) (*Recovery, error) {
	shards := make([]*slots, shardCnt)
	files := []os.File{}
	for i := 0; i < shardCnt; i++ {
		file, err := os.OpenFile(path.Join(dir, fmt.Sprintf("shard_%03d", i)), os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}
		fi, err := file.Stat()
		if err != nil {
			return nil, err
		}
		size := uint32(fi.Size() / int64(datasize))
		ffile, err := os.OpenFile(path.Join(dir, fmt.Sprintf("free_%03d", i)), os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}
		sl := newSlots(size, ffile, shardSize)
		sl.data = make([]byte, size/8)
		sl.size = size
		sl.head = 0
		shards[i] = sl
	}
	return &Recovery{shards, files}, nil
}

func (r *Recovery) Add(loc Location) error {
	sh := r.shards[loc.Shard]
	l := len(sh.data)
	if diff := int(loc.Slot/8) - l; diff >= 0 {
		sh.extend(diff + 1)
		for i := 0; i <= diff; i++ {
			sh.data[l+i] = 0x0
		}
	}
	sh.push(loc.Slot)
	return nil
}

func (r *Recovery) Save() (err error) {
	for _, sh := range r.shards {
		for i := range sh.data {
			sh.data[i] ^= 0xff
		}
		if err := sh.save(); err != nil {
			return err
		}
	}
	for _, file := range r.files {
		err = multierror.Append(err, file.Close())
	}
	if err, ok := err.(*multierror.Error); ok {
		return err.ErrorOrNil()
	}
	return err
	return nil
}
