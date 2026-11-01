#!/usr/bin/env bash
set -euo pipefail

mode=${1:-both}
if [[ "$mode" != stress && "$mode" != fuzz && "$mode" != both ]]; then
  echo 'usage: scripts/stress-informix.sh [stress|fuzz|both]' >&2
  exit 2
fi

IBM_DB_HOME=${IBM_DB_HOME:-/tmp/go_ibm_db_cli_check/clidriver}
if [[ ! -f "$IBM_DB_HOME/include/sqlcli1.h" ]]; then
  echo 'set IBM_DB_HOME to the installed clidriver directory' >&2
  exit 2
fi
export IBM_DB_HOME
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -L$IBM_DB_HOME/lib/odbc"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

if [[ -z "${DB2_STRESS_DSN:-}" ]]; then
  : "${IFX_PASSWORD:?set IFX_PASSWORD or DB2_STRESS_DSN}"
  if [[ -z "${IFX_PORT:-}" ]]; then
    IFX_PORT=$(docker port sendsay-ifx-probe 9089/tcp | sed -n 's/.*://p' | head -n 1)
  fi
  if [[ -z "$IFX_PORT" ]]; then
    echo 'cannot find Informix DRDA port; set IFX_PORT' >&2
    exit 2
  fi
  export DB2_STRESS_DSN="DATABASE=sysmaster;HOSTNAME=127.0.0.1;PORT=$IFX_PORT;PROTOCOL=TCPIP;UID=informix;PWD=$IFX_PASSWORD"
fi
export DB2_STRESS_QUERY=${DB2_STRESS_QUERY:-'SELECT COUNT(*) FROM sysdatabases'}
export DB2_STRESS_ROWS_QUERY=${DB2_STRESS_ROWS_QUERY:-'SELECT tabid FROM systables'}
export DB2_STRESS_PLATFORM=INFORMIX
export DB2_STRESS_DURATION=${DB2_STRESS_DURATION:-2m}
export DB2_STRESS_WORKERS=${DB2_STRESS_WORKERS:-8}
export DB2_STRESS_SEED=${DB2_STRESS_SEED:-1}

test_timeout=${DB2_STRESS_TEST_TIMEOUT:-5m}
if [[ "$mode" == stress || "$mode" == both ]]; then
  go test -race -count=1 -timeout="$test_timeout" -run '^TestLiveStress$' -v ./stress
fi
if [[ "$mode" == fuzz || "$mode" == both ]]; then
  go test -race -count=1 -timeout="$test_timeout" -run '^$' -fuzz '^FuzzLiveSequence$' \
    -fuzztime="${DB2_STRESS_FUZZTIME:-1m}" -parallel="${DB2_STRESS_FUZZ_WORKERS:-4}" ./stress
fi
