package leveldb

var DbSchemaCurrent = dbSchemaCurrent

func (s *Store) GetSchemaName() (string, error) {
	return s.getSchemaName()
}
