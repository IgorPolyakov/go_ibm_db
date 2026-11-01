#!/usr/bin/env bash
# Stage: verification against a real DB2 LUW instance.
# Starts icr.io/db2_community/db2, waits for readiness, then runs the
# testdata suite (array/tx/... tests that Informix skips), the stress
# harness and a differential check against the container's own CLP.
set -euo pipefail

NAME=${DB2_CONTAINER:-db2-luw}
PORT=${DB2_PORT:-50001}
PASSWORD=${DB2_PASSWORD:-db2testpw}
export PATH="$HOME/.local/share/mise/installs/go/1.27.3/bin:$PATH"

IBM_DB_HOME=${IBM_DB_HOME:-/tmp/go_ibm_db_cli_check/clidriver}
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -L$IBM_DB_HOME/lib/odbc"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

DSN="DATABASE=testdb;HOSTNAME=127.0.0.1;PORT=$PORT;PROTOCOL=TCPIP;UID=db2inst1;PWD=$PASSWORD"

if ! docker inspect "$NAME" >/dev/null 2>&1; then
  docker run -d --name "$NAME" -e LICENSE=accept -e DB2INST1_PASSWORD="$PASSWORD" \
    -e DBNAME=testdb -p "$PORT":50000 icr.io/db2_community/db2:latest
fi

echo "waiting for DB2 readiness (first boot takes a few minutes)..."
for i in $(seq 1 120); do
  if docker logs "$NAME" 2>&1 | grep -q 'Setup has completed'; then
    echo "ready after ${i}0s"; break
  fi
  sleep 10
  [ "$i" = 120 ] && { docker logs "$NAME" 2>&1 | tail -20; exit 1; }
done
# extra settle moment for the listener
sleep 15

cd /tmp/go_ibm_db
echo "=== testdata suite vs DB2 LUW:"
make -f - <<'MK' >/dev/null 2>&1 || true
MK
( cd testdata && DB2_CONNSTR="$DSN" DB2_TARGET_PLATFORM=LUW go test -count=1 -v . > /tmp/opencode/db2_suite.log 2>&1 )
echo "FAIL: $(grep -cE '^--- FAIL' /tmp/opencode/db2_suite.log || true)  SKIP: $(grep -cE '^--- SKIP' /tmp/opencode/db2_suite.log || true)  PASS: $(grep -cE '^--- PASS' /tmp/opencode/db2_suite.log || true)"
grep -E '^--- FAIL' /tmp/opencode/db2_suite.log | head -10 || true

echo "=== stress vs DB2 LUW:"
( cd . && DB2_STRESS_DSN="$DSN" DB2_STRESS_QUERY='SELECT COUNT(*) FROM SYSCAT.TABLES' \
  DB2_STRESS_ROWS_QUERY='SELECT 1 FROM SYSCAT.TABLES FETCH FIRST 32 ROWS ONLY' \
  DB2_STRESS_DURATION=60s DB2_STRESS_WORKERS=8 DB2_STRESS_TEST_TIMEOUT=180s \
  go test -race -count=1 -run '^TestLiveStress$' -v ./stress > /tmp/opencode/db2_stress.log 2>&1 )
grep -E -e '^--- ' -e 'handles:' -e 'operations=' /tmp/opencode/db2_stress.log | tail -3
