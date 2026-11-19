#!/usr/bin/env bash
# Compares row counts of a restored database with the live one (test case N-02).
# Usage: compare-counts.sh <live-before.txt> <restored.txt> <live-after.txt>
#
# live-before is taken right before the restore starts (after forcing a WAL switch), live-after
# right after it finishes. Every table must exist in the restored copy and satisfy
#     live-before <= restored <= live-after
# i.e. nothing that existed before the test is missing, and nothing appears from nowhere.
# On a quiet system (night, CI) all three numbers are equal.
set -euo pipefail
[ $# -eq 3 ] || { echo "usage: $0 live-before.txt restored.txt live-after.txt" >&2; exit 2; }

awk '
  FILENAME == ARGV[1] { before[$1] = $2; next }
  FILENAME == ARGV[2] { restored[$1] = $2; next }
  FILENAME == ARGV[3] { after[$1] = $2; next }
  END {
    bad = 0; n = 0
    printf "%-40s %12s %12s %12s  %s\n", "table", "live-before", "restored", "live-after", "result"
    for (t in before) {
      n++
      if (!(t in restored)) { printf "%-40s %12s %12s %12s  %s\n", t, before[t], "-", after[t], "MISSING"; bad++; continue }
      a = (t in after) ? after[t] : before[t]
      ok = (restored[t] + 0 >= before[t] + 0) && (restored[t] + 0 <= a + 0)
      if (!ok) bad++
      printf "%-40s %12d %12d %12d  %s\n", t, before[t], restored[t], a, ok ? "OK" : "MISMATCH"
    }
    for (t in restored) if (!(t in before)) { printf "%-40s %12s %12d %12s  %s\n", t, "-", restored[t], "-", "EXTRA"; bad++ }
    if (n == 0) { print "no tables found in the live database"; exit 1 }
    if (bad > 0) { printf "N-02 FAILED: %d table(s) differ\n", bad; exit 1 }
    printf "N-02 OK: %d tables match\n", n
  }
' "$1" "$2" "$3"
