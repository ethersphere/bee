package sqlite

import (
	"encoding"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormLogger "gorm.io/gorm/logger"
)

type keyValue struct {
	Key   string `gorm:"primaryKey;index"`
	Value []byte `gorm:"type:blob"`
}

type statestore struct {
	db     *gorm.DB
	logger logging.Logger
}

func NewStateStore(logger logging.Logger, dataDir string) (*statestore, error) {

	newLogger := gormLogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormLogger.Config{
			SlowThreshold:             time.Second,     // Slow SQL threshold
			LogLevel:                  gormLogger.Warn, // Log level
			IgnoreRecordNotFoundError: true,            // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,           // Disable color
		},
	)

	db, err := gorm.Open(sqlite.Open(path.Join(dataDir, "sqlite")), &gorm.Config{
		// SkipDefaultTransaction: true,
		// PrepareStmt:            false,
		Logger: newLogger,
		// Logger: gormLogger.Default.LogMode(gormLogger.Error),
	})

	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&keyValue{})
	if err != nil {
		return nil, err
	}

	err = db.Exec("PRAGMA synchronous = OFF").Error
	if err != nil {
		return nil, err
	}
	err = db.Exec("PRAGMA journal_mode = OFF").Error
	if err != nil {
		return nil, err
	}

	// err = db.Exec("create unique index index_key on key_values (key)").Error
	// if err != nil {
	// 	return nil, err
	// }

	return &statestore{
		db:     db,
		logger: logger,
	}, nil
}

func (s *statestore) Put(key string, i interface{}) (err error) {
	var value []byte

	if marshaler, ok := i.(encoding.BinaryMarshaler); ok {
		if value, err = marshaler.MarshalBinary(); err != nil {
			return err
		}
	} else if value, err = json.Marshal(i); err != nil {
		return err
	}

	// s.logger.Infof("putting %s %T %s", key, i, string(value))

	return s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&keyValue{Key: key, Value: value}).Error
}

func (s *statestore) Get(key string, i interface{}) error {

	// s.logger.Infof("getting %s %T", key, i)

	kv := &keyValue{
		Key: key,
	}

	if err := s.db.Take(kv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storage.ErrNotFound
		}
		return err
	}

	if unmarshaler, ok := i.(encoding.BinaryUnmarshaler); ok {
		return unmarshaler.UnmarshalBinary(kv.Value)
	}

	// s.logger.Infof("getting %s %s", key, string(kv.Value))

	return json.Unmarshal(kv.Value, i)

}

func (s *statestore) Delete(key string) error {
	return s.db.Delete(&keyValue{Key: key}).Error
}

func (s *statestore) Iterate(prefix string, iter storage.StateIterFunc) error {

	rows, err := s.db.Model(&keyValue{}).Where("key MATCH ?", prefix+"*").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var kv keyValue
		// ScanRows is a method of `gorm.DB`, it can be used to scan a row into a struct
		err := s.db.ScanRows(rows, &kv)
		if err != nil {
			return err
		}

		if stop, err := iter([]byte(kv.Key), kv.Value); stop || err != nil {
			return err
		}
	}

	return nil
}

func (s *statestore) DB() *leveldb.DB {
	panic("leveldb not found")
	return nil
}

func (s *statestore) Migrate(oldS storage.StateStorer) error {
	// return nil
	return s.db.Transaction(func(tx *gorm.DB) error {
		return oldS.Iterate("", func(key, value []byte) (stop bool, err error) {
			// s.logger.Infof("putting %s %s", key, string(value))
			return false, tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&keyValue{Key: string(key), Value: value}).Error
		})
	})
}

func (s *statestore) Close() error {
	return nil
}

// type StateStorer interface {
// 	Get(key string, i interface{}) (err error)
// 	Put(key string, i interface{}) (err error)
// 	Delete(key string) (err error)
// 	Iterate(prefix string, iterFunc StateIterFunc) (err error)
// 	// DB returns the underlying DB storage.
// 	DB() *leveldb.DB
// 	io.Closer
// }
