package leveldb

var DbSchemaCurrent = dbSchemaCurrent

func (s *store) GetSchemaName() (string, error) {
	return s.getSchemaName()
}
