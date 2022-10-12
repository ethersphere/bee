package migration

func NewOptions() *options {
	return newOptions()
}

func (o *options) DeleteFn() ItemDeleteFn {
	return o.deleteFn
}

func (o *options) UpdateFn() ItemUpdateFn {
	return o.updateFn
}

func (o *options) ApplyAll(opt ...option) {
	o.applyAll(opt)
}
