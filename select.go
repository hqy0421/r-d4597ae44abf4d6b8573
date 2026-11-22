package sqlittle

import (
	"errors"

	sdb "github.com/alicebob/sqlittle/db"
)

func select_(db *sdb.Database, s *sdb.Schema, cb RowSourceDoneCB, columns []string) error {
	ci, err := toColumnIndexRowid(s, columns)
	if err != nil {
		return err
	}

	t, err := db.Table(s.Table)
	if err != nil {
		return err
	}
	return t.Scan(func(rowid int64, r sdb.Record) bool {
		row, src := toRowSource(rowid, ci, r)
		return cb(row, src)
	})
}

func selectNonRowid(db *sdb.Database, s *sdb.Schema, cb RowSourceDoneCB, columns []string) error {
	ci, err := toColumnIndexNonRowid(s, columns)
	if err != nil {
		return err
	}

	t, err := db.NonRowidTable(s.Table)
	if err != nil {
		return err
	}
	return t.Scan(func(r sdb.Record) bool {
		row, src := toRowSource(0, ci, r)
		return cb(row, src)
	})
}

func selectRowid(db *sdb.Database, s *sdb.Schema, rowid int64, columns []string) (Row, []ValueSource, error) {
	ci, err := toColumnIndexRowid(s, columns)
	if err != nil {
		return nil, nil, err
	}

	t, err := db.Table(s.Table)
	if err != nil {
		return nil, nil, err
	}
	r, err := t.Rowid(rowid)
	if err != nil || r == nil {
		return nil, nil, err
	}
	// TODO: decide what to do with shared []byte pointers
	row, src := toRowSource(rowid, ci, r)
	return row, src, nil
}

func pkSelect(db *sdb.Database, s *sdb.Schema, key Key, cb RowSourceCB, columns []string) error {
	if s.RowidPK {
		// `integer primary key` table.
		var rowid int64
		if len(key) == 0 {
			return errors.New("invalid key")
		}
		rowid, ok := key[0].(int64)
		if !ok {
			return errors.New("invalid key")
		}
		row, src, err := selectRowid(db, s, rowid, columns)
		if err != nil {
			return err
		}
		if row != nil {
			cb(row, src)
		}
		return nil
	}
	ind := s.NamedIndex(s.PrimaryKey)
	if ind == nil {
		return errors.New("table has no primary key")
	}

	dbkey, err := asDbKey(key, ind.Columns)
	if err != nil {
		return err
	}

	return indexedSelectEq(
		db,
		s,
		ind,
		dbkey,
		cb,
		columns,
	)
}

func pkSelectNonRowid(db *sdb.Database, s *sdb.Schema, key Key, cb RowSourceCB, columns []string) error {
	ci, err := toColumnIndexNonRowid(s, columns)
	if err != nil {
		return err
	}
	t, err := db.NonRowidTable(s.Table)
	if err != nil {
		return err
	}

	dbkey, err := asDbKey(key, s.PK)
	if err != nil {
		return err
	}

	return t.ScanEq(
		dbkey,
		func(r sdb.Record) bool {
			row, src := toRowSource(0, ci, r)
			cb(row, src)
			return false
		},
	)
}
