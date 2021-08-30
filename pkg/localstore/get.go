package localstore

func (d *DB) GcSize() uint64 {
	v, _ := d.gcSize.Get()
	return v
}

func (d *DB) ReserveSize() uint64 {
	v, _ := d.reserveSize.Get()
	return v
}
