package sqlittle

import (
	"fmt"
	"strings"

	sdb "github.com/alicebob/sqlittle/db"
)

type columnIndex struct {
	col      *sdb.TableColumn
	rowIndex int
	rowid    bool
}

// ValueSource describes where a single value in a Row came from.
//
// SQLite stores a row as a record holding exactly the columns the table had
// when the row was written. When columns were added later with
// `ALTER TABLE ... ADD COLUMN` the older rows are physically shorter, and
// SQLite reads those missing columns as the column's DEFAULT (or NULL when
// the column has no default). That effective value is indistinguishable from
// a stored value -- and a stored NULL is indistinguishable from a missing
// column without a default -- by looking at the value alone. The source is
// therefore decided while the record is decoded, from the number of columns
// the record actually stores, never from the value.
//
// Table scans and index lookups decode the very same table records through
// the same code path, so both report the same source for the same row.
type ValueSource int

const (
	// SourceStored: the value was physically stored in the row's record.
	SourceStored ValueSource = iota
	// SourceStoredNull: a NULL was physically stored in the row's record.
	SourceStoredNull
	// SourceDefault: the row's record is too short to hold this column: the
	// row predates the `ALTER TABLE ... ADD COLUMN` which introduced it.
	// The value is the column's DEFAULT, or NULL when it has none.
	SourceDefault
	// SourceRowid: the value is the row's rowid; it is never stored in the
	// record. Used for an `INTEGER PRIMARY KEY` column and for the
	// rowid/oid/_rowid_ pseudo columns.
	SourceRowid
)

func (s ValueSource) String() string {
	switch s {
	case SourceStored:
		return "stored"
	case SourceStoredNull:
		return "stored NULL"
	case SourceDefault:
		return "column default"
	case SourceRowid:
		return "rowid"
	default:
		return fmt.Sprintf("ValueSource(%d)", int(s))
	}
}

// Regroups a Record to a Row, filling in missing columns as needed.
//
// The returned sources have the same length as the Row and describe where
// every value came from. They are derived from the record's stored column
// count, never from the values themselves.
func toRowSource(rowid int64, cis []columnIndex, r sdb.Record) (Row, []ValueSource) {
	row := make(Row, len(cis))
	src := make([]ValueSource, len(cis))
	for i, c := range cis {
		switch {
		case c.rowid:
			row[i] = rowid
			src[i] = SourceRowid
		case len(r) <= c.rowIndex:
			// use 'DEFAULT' when the record is too short
			row[i] = c.col.Default
			src[i] = SourceDefault
		case r[c.rowIndex] == nil:
			row[i] = nil
			src[i] = SourceStoredNull
		default:
			row[i] = r[c.rowIndex]
			src[i] = SourceStored
		}
	}
	return row, src
}

// given column names returns the index in a Row this column is expected, and
// the column definition. Allows 'rowid' alias.
func toColumnIndexRowid(s *sdb.Schema, columns []string) ([]columnIndex, error) {
	res := make([]columnIndex, 0, len(columns))
	for _, c := range columns {
		n := s.Column(c)
		if n < 0 {
			cup := strings.ToUpper(c)
			if cup == "ROWID" || cup == "OID" || cup == "_ROWID_" {
				res = append(res, columnIndex{nil, n, true})
				continue
			} else {
				return nil, fmt.Errorf("no such column: %q", c)
			}
		}
		c := &s.Columns[n]
		if c.Rowid {
			res = append(res, columnIndex{nil, n, true})
		} else {
			res = append(res, columnIndex{c, n, false})
		}
	}
	return res, nil
}

// given column names returns the index of this column in a row in the index (and
// the column definition). For non-rowid tables the database order of the
// columns depends on the primary key.
func toColumnIndexNonRowid(s *sdb.Schema, columns []string) ([]columnIndex, error) {
	stored := columnStoreOrder(s) // column indexes in disk order
	res := make([]columnIndex, 0, len(columns))
	for _, c := range columns {
		n := s.Column(c)
		if n < 0 {
			return nil, fmt.Errorf("no such column: %q", c)
		}
		res = append(res, columnIndex{&s.Columns[n], stored[n], false})
	}
	return res, nil
}

// given a non-rowid table, gives the order columns are stored on disk
func columnStoreOrder(schema *sdb.Schema) []int {
	if !schema.WithoutRowid {
		panic("can't call columnStoreOrder on a rowid table")
	}

	// all PK columns come first, then all other columns, in order
	var cols = make([]string, 0, len(schema.Columns))
	for _, c := range schema.PK {
		cols = append(cols, strings.ToLower(c.Column))
	}
loop:
	for _, c := range schema.Columns {
		n := strings.ToLower(c.Column)
		for _, oc := range cols {
			if oc == n {
				continue loop
			}
		}
		cols = append(cols, n)
	}

	res := make([]int, len(cols))
loop2:
	for i, c := range schema.Columns {
		n := strings.ToLower(c.Column)
		for j, oc := range cols {
			if oc == n {
				res[i] = j
				continue loop2
			}
		}
	}
	return res
}

// for non-rowid tables only:
// given an index gives back the indexes in a row which form the primary key.
func pkColumns(schema *sdb.Schema, ind *sdb.SchemaIndex) []int {
	if !schema.WithoutRowid {
		panic("can't call pkColumns on a rowid table")
	}

	var res []int
	for _, c := range schema.PK {
		if in := ind.Column(c.Column); in < 0 {
			ind.Columns = append(ind.Columns, c)
			res = append(res, len(ind.Columns)-1)
		} else {
			res = append(res, in)
		}
	}
	return res
}
