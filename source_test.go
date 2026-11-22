package sqlittle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// a row and the source of every value in it
type sourcedRow struct {
	row Row
	src []ValueSource
}

func collectSource(t *testing.T, db *DB, table string, columns ...string) []sourcedRow {
	t.Helper()
	var rows []sourcedRow
	err := db.SelectWithSource(table, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, columns...)
	require.NoError(t, err)
	return rows
}

func TestSelectWithSource(t *testing.T) {
	// The readings table was ALTERed twice: `scale` was added with DEFAULT
	// 1.5 when only rows 1-2 existed, and `flag` was added with DEFAULT 'ok'
	// when rows 1-5 existed.
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	rows := collectSource(t, db, "readings", "id", "name", "scale", "flag")
	require.Equal(t,
		[]sourcedRow{
			// rows written before the ALTERs read as the defaults
			{Row{int64(1), "alpha", 1.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceDefault, SourceDefault}},
			{Row{int64(2), "beta", 1.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceDefault, SourceDefault}},
			{Row{int64(3), "gamma", 2.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceStored, SourceDefault}},
			// an explicit NULL is not a missing column
			{Row{int64(4), "delta", nil, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceStoredNull, SourceDefault}},
			// a stored 1.5 is not a default, even though the value is equal
			{Row{int64(5), "echo", 1.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceStored, SourceDefault}},
			{Row{int64(6), "foxtrot", 3.5, "bad"}, []ValueSource{SourceRowid, SourceStored, SourceStored, SourceStored}},
			{Row{int64(7), "golf", nil, nil}, []ValueSource{SourceRowid, SourceStored, SourceStoredNull, SourceStoredNull}},
			// same values as row 1, but everything was really stored
			{Row{int64(8), "hotel", 1.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceStored, SourceStored}},
		},
		rows,
	)
}

func TestSelectWithSourceNotGuessedFromValue(t *testing.T) {
	// Rows 1, 5, and 8 all read scale as 1.5, but the source differs: it
	// comes from the record layout, never from the value.
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	sources := map[int64]ValueSource{}
	values := map[int64]interface{}{}
	err = db.SelectWithSource("readings", func(r Row, src []ValueSource) {
		id := r[0].(int64)
		values[id] = r[1]
		sources[id] = src[1]
	}, "id", "scale")
	require.NoError(t, err)

	require.Equal(t, 1.5, values[1])
	require.Equal(t, 1.5, values[5])
	require.Equal(t, 1.5, values[8])
	require.Equal(t, SourceDefault, sources[1]) // row predates the column
	require.Equal(t, SourceStored, sources[5])  // 1.5 was really stored
	require.Equal(t, SourceStored, sources[8])  // 1.5 was really stored
}

func TestSelectRowidWithSource(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	for rowid, want := range map[int64]sourcedRow{
		1: {Row{int64(1), int64(1), 1.5, "ok"}, []ValueSource{SourceRowid, SourceRowid, SourceDefault, SourceDefault}},
		4: {Row{int64(4), int64(4), nil, "ok"}, []ValueSource{SourceRowid, SourceRowid, SourceStoredNull, SourceDefault}},
		7: {Row{int64(7), int64(7), nil, nil}, []ValueSource{SourceRowid, SourceRowid, SourceStoredNull, SourceStoredNull}},
		8: {Row{int64(8), int64(8), 1.5, "ok"}, []ValueSource{SourceRowid, SourceRowid, SourceStored, SourceStored}},
	} {
		row, src, err := db.SelectRowidWithSource("readings", rowid, "id", "rowid", "scale", "flag")
		require.NoError(t, err)
		require.Equal(t, want, sourcedRow{row, src}, "rowid %d", rowid)
	}

	// no such rowid
	row, src, err := db.SelectRowidWithSource("readings", 999, "id")
	require.NoError(t, err)
	require.Nil(t, row)
	require.Nil(t, src)
}

func TestIndexedSelectWithSourceMatchesTableScan(t *testing.T) {
	// The index only provides the row order; values and sources come from the
	// data table, so an index scan must report the same sources as a table
	// scan.
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	columns := []string{"id", "name", "scale", "flag"}

	scan := map[int64]sourcedRow{}
	for _, sr := range collectSource(t, db, "readings", columns...) {
		scan[sr.row[0].(int64)] = sr
	}
	require.Len(t, scan, 8)

	indexed := map[int64]sourcedRow{}
	err = db.IndexedSelectWithSource("readings", "readings_name", func(r Row, src []ValueSource) {
		indexed[r[0].(int64)] = sourcedRow{r, src}
	}, columns...)
	require.NoError(t, err)

	require.Equal(t, scan, indexed)
}

func TestIndexedSelectEqWithSource(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	var rows []sourcedRow
	err = db.IndexedSelectEqWithSource("readings", "readings_name", Key{"delta"}, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, "id", "scale", "flag")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{int64(4), nil, "ok"}, []ValueSource{SourceRowid, SourceStoredNull, SourceDefault}},
		},
		rows,
	)

	rows = nil
	err = db.IndexedSelectEqWithSource("readings", "readings_name", Key{"hotel"}, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, "id", "scale", "flag")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{int64(8), 1.5, "ok"}, []ValueSource{SourceRowid, SourceStored, SourceStored}},
		},
		rows,
	)
}

func TestPKSelectWithSource(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	var rows []sourcedRow
	err = db.PKSelectWithSource("readings", Key{int64(7)}, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, "id", "scale", "flag")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{int64(7), nil, nil}, []ValueSource{SourceRowid, SourceStoredNull, SourceStoredNull}},
		},
		rows,
	)

	rows = nil
	err = db.PKSelectWithSource("readings", Key{int64(2)}, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, "id", "scale", "flag")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{int64(2), 1.5, "ok"}, []ValueSource{SourceRowid, SourceDefault, SourceDefault}},
		},
		rows,
	)
}

func TestSelectWithSourceNonRowid(t *testing.T) {
	// Same history, but as a WITHOUT ROWID table.
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	rows := collectSource(t, db, "readings_wr", "name", "scale", "flag")
	require.Equal(t,
		[]sourcedRow{
			{Row{"alpha", 1.5, "ok"}, []ValueSource{SourceStored, SourceStored, SourceDefault}},
			{Row{"beta", 2.5, "bad"}, []ValueSource{SourceStored, SourceStored, SourceStored}},
			{Row{"delta", 1.5, "ok"}, []ValueSource{SourceStored, SourceStored, SourceStored}},
			{Row{"gamma", nil, nil}, []ValueSource{SourceStored, SourceStoredNull, SourceStoredNull}},
		},
		rows,
	)
}

func TestIndexedSelectWithSourceNonRowidMatchesTableScan(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	columns := []string{"name", "scale", "flag"}

	scan := map[string]sourcedRow{}
	for _, sr := range collectSource(t, db, "readings_wr", columns...) {
		scan[sr.row[0].(string)] = sr
	}
	require.Len(t, scan, 4)

	indexed := map[string]sourcedRow{}
	err = db.IndexedSelectWithSource("readings_wr", "readings_wr_scale", func(r Row, src []ValueSource) {
		indexed[r[0].(string)] = sourcedRow{r, src}
	}, columns...)
	require.NoError(t, err)

	require.Equal(t, scan, indexed)
}

func TestPKSelectWithSourceNonRowid(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	var rows []sourcedRow
	err = db.PKSelectWithSource("readings_wr", Key{"gamma"}, func(r Row, src []ValueSource) {
		rows = append(rows, sourcedRow{r, src})
	}, "name", "scale", "flag")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{"gamma", nil, nil}, []ValueSource{SourceStored, SourceStoredNull, SourceStoredNull}},
		},
		rows,
	)
}

func TestSelectDoneWithSource(t *testing.T) {
	db, err := Open("testdata/alter_history.sqlite")
	require.NoError(t, err)
	defer db.Close()

	var rows []sourcedRow
	err = db.SelectDoneWithSource("readings", func(r Row, src []ValueSource) bool {
		rows = append(rows, sourcedRow{r, src})
		return len(rows) == 2
	}, "id", "scale")
	require.NoError(t, err)
	require.Equal(t,
		[]sourcedRow{
			{Row{int64(1), 1.5}, []ValueSource{SourceRowid, SourceDefault}},
			{Row{int64(2), 1.5}, []ValueSource{SourceRowid, SourceDefault}},
		},
		rows,
	)
}

func TestSelectWithSourceNoAlter(t *testing.T) {
	// a table which was never ALTERed only has stored values
	db, err := Open("testdata/words.sqlite")
	require.NoError(t, err)
	defer db.Close()

	count := 0
	err = db.SelectDoneWithSource("words", func(r Row, src []ValueSource) bool {
		require.Equal(t, []ValueSource{SourceStored, SourceStored, SourceRowid}, src)
		count++
		return count >= 10
	}, "word", "length", "rowid")
	require.NoError(t, err)
	require.Equal(t, 10, count)
}

func TestValueSourceString(t *testing.T) {
	require.Equal(t, "stored", SourceStored.String())
	require.Equal(t, "stored NULL", SourceStoredNull.String())
	require.Equal(t, "column default", SourceDefault.String())
	require.Equal(t, "rowid", SourceRowid.String())
	require.Equal(t, "ValueSource(42)", ValueSource(42).String())
}
