#!/usr/bin/env bash
# Differential testing: the same queries through the Go driver and through
# isql (unixODBC -> the same libdb2, but a completely different client
# stack above it); outputs are normalized and diffed.
#
# Prereqs: ~/.odbc.ini entry "ifx" (or override IFX_DSN below),
# db2dsdriver.cfg with the matching alias, LD_LIBRARY_PATH to clidriver.
set -euo pipefail

IBM_DB_HOME=${IBM_DB_HOME:-/tmp/go_ibm_db_cli_check/clidriver}
IFX_DSN=${IFX_DSN:-ifx}
IFX_USER=${IFX_USER:-informix}
IFX_PASS=${IFX_PASSWORD:?set IFX_PASSWORD}
GO_DSN=${GO_DSN:-"DATABASE=sysmaster;HOSTNAME=127.0.0.1;PORT=19089;PROTOCOL=TCPIP;UID=$IFX_USER;PWD=$IFX_PASS"}
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -L$IBM_DB_HOME/lib/odbc"

cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# isql reads statements line by line: keep every statement single-line
# reference data with edge values across types
timeout 60 isql "$IFX_DSN" "$IFX_USER" "$IFX_PASS" <<'SETUP' >/dev/null 2>&1
DROP TABLE godiff;
CREATE TABLE godiff (id INTEGER NOT NULL PRIMARY KEY, big BIGINT, dec DECIMAL(16,6), flt FLOAT, vc VARCHAR(64), ch CHAR(8), d DATE, ts DATETIME YEAR TO SECOND);
INSERT INTO godiff VALUES (1,  9223372036854775807,  1234567890.123456,  3.14159265358979, 'plain',        'abc',     '2024-02-29', '2024-02-29 23:59:59');
INSERT INTO godiff VALUES (2, -9223372036854775807, -9999999999.999999, -2.71828182845905, 'ascii-only',     'pad',     '1999-12-31', '1999-12-31 00:00:00');
INSERT INTO godiff VALUES (3,  0,  0.000001, 0, '', 'x', '2000-01-01', '2000-01-01 12:00:00');
INSERT INTO godiff VALUES (4,  NULL, NULL, NULL, NULL, NULL, NULL, NULL);
COMMIT;
SETUP

queries=(
  "SELECT id, big, dec, flt, vc, ch, d, ts FROM godiff WHERE id < 4 ORDER BY id"
  "SELECT COUNT(*), MIN(big), MAX(big) FROM godiff"
  "SELECT SUM(dec), AVG(flt) FROM godiff WHERE id < 4"
  "SELECT id, LENGTH(vc) FROM godiff WHERE id < 4 ORDER BY id"
  "SELECT id, vc IS NULL, ch IS NULL, d IS NULL FROM godiff ORDER BY id"
  "SELECT tabid, tabname FROM systables WHERE tabid <= 5 ORDER BY tabid"
)

go build -o "$work/dumpquery" ./scripts/diff/dumpquery

fails=0
i=0
for q in "${queries[@]}"; do
  i=$((i+1))
  {
    printf '%s\nquit\n' "$q"
  } | timeout 60 isql "$IFX_DSN" "$IFX_USER" "$IFX_PASS" -d '|' \
    | awk -F'|' '{ gsub(/^\|/, "", $0); gsub(/\|$/, "", $0); NF2=split($0, f, "|"); out=""; for (i=1; i<=NF2; i++) { gsub(/^ +| +$/, "", f[i]); if (f[i] == "") f[i] = "\\N"; out = out (i>1 ? "|" : "") f[i] } print out }' \
    | grep -vE '^(SQL>|\+|Connected!|sql-statement|help \[tablename\]|echo \[string\]|quit|SQLRowCount|[0-9]+ rows fetched|$)' \
    | grep -v '^\\N$' | tail -n +2 > "$work/isql.$i"
  "$work/dumpquery" -dsn "$GO_DSN" -q "$q" > "$work/go.$i"
  if python3 - "$work/isql.$i" "$work/go.$i" <<'PYCMP'
import sys
def rows(path):
    out = []
    for line in open(path):
        fields = []
        for f in line.rstrip("\n").split("|"):
            f = f.strip().replace(",", ".")
            if f == "\\N":
                f = ""   # isql renders NULL and '' identically
            try:
                f = float(f)
            except ValueError:
                f = f.rstrip()
            fields.append(f)
        out.append(fields)
    return out
a, b = rows(sys.argv[1]), rows(sys.argv[2])
if len(a) != len(b):
    print(f"row count {len(a)} vs {len(b)}"); sys.exit(1)
for ra, rb in zip(a, b):
    if len(ra) != len(rb):
        print("field count mismatch", ra, rb); sys.exit(1)
    for xa, xb in zip(ra, rb):
        if isinstance(xa, float) and isinstance(xb, float):
            if abs(xa - xb) > 1e-9 * max(abs(xa), abs(xb), 1e-300):
                print("numeric mismatch", xa, xb); sys.exit(1)
        elif isinstance(xa, str) and isinstance(xb, str):
            # DATE vs DATETIME rendering: compare the date prefix
            if len(xa) >= 10 and len(xb) >= 10 and xa[:10] == xb[:10] and (
                    xa[10:] in ("", " 00:00:00.000000") or xb[10:] in ("", " 00:00:00.000000")):
                continue
            if xa != xb:
                print("value mismatch", repr(xa), repr(xb)); sys.exit(1)
        elif xa != xb:
            print("value mismatch", repr(xa), repr(xb)); sys.exit(1)
PYCMP
  then
    echo "query $i: OK ($(wc -l < "$work/go.$i") rows)"
  else
    echo "query $i: MISMATCH"
    diff -u "$work/isql.$i" "$work/go.$i" | sed -n '1,12p'
    fails=$((fails+1))
  fi
done

if [ "$fails" -gt 0 ]; then
  echo "$fails of ${#queries[@]} queries diverged" >&2
  exit 1
fi
echo "all ${#queries[@]} queries identical"
