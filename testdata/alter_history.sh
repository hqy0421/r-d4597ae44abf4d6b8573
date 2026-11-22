#!/bin/bash
set -eu

DB="alter_history.sqlite"
rm -f $DB
(
    # v1 of the readings table
    echo "CREATE TABLE readings (id INTEGER PRIMARY KEY, name TEXT);"
    echo "INSERT INTO readings VALUES (1, 'alpha');"
    echo "INSERT INTO readings VALUES (2, 'beta');"
    # v2 adds a column with a default; rows 1 and 2 stay physically short
    echo "ALTER TABLE readings ADD COLUMN scale REAL DEFAULT 1.5;"
    echo "INSERT INTO readings VALUES (3, 'gamma', 2.5);"
    echo "INSERT INTO readings VALUES (4, 'delta', NULL);"
    # stored 1.5: equal to the default, but really stored
    echo "INSERT INTO readings VALUES (5, 'echo', 1.5);"
    # v3 adds another column with a default; rows 1-5 stay short here
    echo "ALTER TABLE readings ADD COLUMN flag TEXT DEFAULT 'ok';"
    echo "INSERT INTO readings VALUES (6, 'foxtrot', 3.5, 'bad');"
    echo "INSERT INTO readings VALUES (7, 'golf', NULL, NULL);"
    # stored 1.5 and 'ok': equal to the defaults, but really stored
    echo "INSERT INTO readings VALUES (8, 'hotel', 1.5, 'ok');"
    echo "CREATE INDEX readings_name ON readings(name);"

    # the same history, as a WITHOUT ROWID table
    echo "CREATE TABLE readings_wr (name TEXT PRIMARY KEY, scale REAL) WITHOUT ROWID;"
    echo "INSERT INTO readings_wr VALUES ('alpha', 1.5);"
    echo "ALTER TABLE readings_wr ADD COLUMN flag TEXT DEFAULT 'ok';"
    echo "INSERT INTO readings_wr VALUES ('beta', 2.5, 'bad');"
    echo "INSERT INTO readings_wr VALUES ('gamma', NULL, NULL);"
    echo "INSERT INTO readings_wr VALUES ('delta', 1.5, 'ok');"
    echo "CREATE INDEX readings_wr_scale ON readings_wr(scale);"
) | sqlite3 --batch $DB
