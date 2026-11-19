#!/usr/bin/env bash
# Prints "<schema.table> <exact row count>" for every user table, sorted.
# Usage: row-counts.sh <postgres connection URI>     (or no argument to use PG* env vars)
set -euo pipefail
psql ${1:+"$1"} -X -A -t -F ' ' -v ON_ERROR_STOP=1 <<'SQL'
SELECT table_schema || '.' || table_name,
       (xpath('/row/c/text()',
              query_to_xml(format('SELECT count(*) AS c FROM %I.%I', table_schema, table_name), false, true, '')))[1]::text::bigint
FROM information_schema.tables
WHERE table_type = 'BASE TABLE'
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY 1;
SQL
