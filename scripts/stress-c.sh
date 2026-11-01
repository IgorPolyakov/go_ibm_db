#!/usr/bin/env bash
# Build and run the C stress harness under ThreadSanitizer and Valgrind.
# It verifies that the driver's DB2 CLI call pattern (parallel workers +
# cross-thread SQLCancel) is thread-safe at the C level. libdb2 itself is
# a closed binary and cannot be instrumented: TSan still intercepts its
# pthread operations, and reports touching our own harness code are real.
set -euo pipefail

IBM_DB_HOME=${IBM_DB_HOME:-/tmp/go_ibm_db_cli_check/clidriver}
: "${IFX_PASSWORD:?set IFX_PASSWORD}"
IFX_PORT=${IFX_PORT:-$(docker port sendsay-ifx-probe 9089/tcp | sed -n 's/.*://p' | head -n 1)}
DSN=${DB2_STRESS_DSN:-"DATABASE=sysmaster;HOSTNAME=127.0.0.1;PORT=$IFX_PORT;PROTOCOL=TCPIP;UID=informix;PWD=$IFX_PASSWORD"}
THREADS=${STRESS_C_THREADS:-4}
SECONDS_=${STRESS_C_SECONDS:-20}
MODE=${1:-tsan}

cd "$(dirname "$0")/.."
out=/tmp/stress_c_bin
case "$MODE" in
  tsan)
    command -v clang >/dev/null || { echo 'clang not found (needed: TSan for C)' >&2; exit 2; }
    clang -O1 -g -fsanitize=thread -fno-omit-frame-pointer \
      -I"$IBM_DB_HOME/include" stress/stress_c.c \
      -L"$IBM_DB_HOME/lib" -ldb2 -o "$out"
    TSAN_OPTIONS="suppressions=$(dirname "$0")/tsan.suppress" \
      LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc" "$out" "$DSN" "$THREADS" "$SECONDS_"
    ;;
  valgrind)
    command -v valgrind >/dev/null || { echo 'valgrind not found' >&2; exit 2; }
    gcc -O1 -g -I"$IBM_DB_HOME/include" stress/stress_c.c \
      -L"$IBM_DB_HOME/lib" -ldb2 -o "$out"
    LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc" \
      valgrind --error-exitcode=42 --leak-check=full --errors-for-leak-kinds=definite \
      --suppressions="$(dirname "$0")/valgrind.suppress" \
      "$out" "$DSN" "$THREADS" "$SECONDS_"
    ;;
  plain)
    gcc -O2 -I"$IBM_DB_HOME/include" stress/stress_c.c -L"$IBM_DB_HOME/lib" -ldb2 -o "$out"
    LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$IBM_DB_HOME/lib/odbc" "$out" "$DSN" "$THREADS" "$SECONDS_"
    ;;
  *) echo 'usage: scripts/stress-c.sh [tsan|valgrind|plain]' >&2; exit 2 ;;
esac
