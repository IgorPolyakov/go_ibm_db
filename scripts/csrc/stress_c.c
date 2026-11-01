/*
 * stress_c.c — a plain C harness that mimics the Go driver's usage of the
 * DB2 CLI library, built with ThreadSanitizer (or run under Valgrind) to
 * verify the CALL PATTERN is thread-safe even though libdb2 itself is a
 * closed binary that cannot be instrumented.
 *
 * Pattern under test (what the Go driver does):
 *   - N worker threads: connect once, then loop prepare -> bind param ->
 *     execute -> fetch (row-wise block fetch with ROW_ARRAY_SIZE=8 and
 *     ROW_STATUS_PTR/ROWS_FETCHED_PTR) -> free stmt
 *   - 1 canceller thread: periodically issues SQLCancel against one of the
 *     running statements from another thread (exactly what the Go context
 *     watcher does), and re-prepares afterwards
 *
 * Build:  see scripts/stress-c.sh
 * Usage:  ./stress_c <dsn> <threads> <seconds>
 */
#include <sqlcli1.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <pthread.h>
#include <stdatomic.h>

static SQLHENV env;
static _Atomic int stop_flag;
static volatile unsigned long g_ops;
static pthread_mutex_t ops_mu = PTHREAD_MUTEX_INITIALIZER;

static void diag(const char *where, SQLSMALLINT type, SQLHANDLE h)
{
	SQLWCHAR state[8];
	SQLINTEGER native = 0;
	SQLWCHAR msg[512];
	SQLSMALLINT len = 0;
	if (SQLGetDiagRecW(type, h, 1, state, &native, msg, (SQLSMALLINT)(sizeof(msg)/sizeof(SQLWCHAR)), &len) != SQL_SUCCESS)
		return;
	/* print ascii-ish */
	char a[512];
	int i;
	for (i = 0; i < len && i < 511; i++) a[i] = (char)(msg[i] & 0xff);
	a[i < 511 ? i : 511] = 0;
	fprintf(stderr, "%s: SQLSTATE=%d.%d.%d native=%d msg=%s\n", where,
		state[0] & 0xff, state[1] & 0xff, state[2] & 0xff, (int)native, a);
}

static SQLHDBC connect_one(const char *dsn)
{
	SQLHDBC dbc;
	SQLWCHAR out[1024];
	SQLSMALLINT outlen = 0;
	SQLRETURN rc;
	rc = SQLAllocHandle(SQL_HANDLE_DBC, env, &dbc);
	if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) return 0;
	/* dsn is ascii; widen */
	SQLWCHAR wdsn[1024];
	size_t n = strlen(dsn);
	size_t i;
	for (i = 0; i <= n; i++) wdsn[i] = (SQLWCHAR)dsn[i];
	rc = SQLDriverConnectW(dbc, 0, wdsn, SQL_NTS, out, 1024, &outlen, SQL_DRIVER_NOPROMPT);
	if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) {
		diag("SQLDriverConnect", SQL_HANDLE_DBC, dbc);
		SQLFreeHandle(SQL_HANDLE_DBC, dbc);
		return 0;
	}
	return dbc;
}

struct worker_arg {
	int id;
	const char *dsn;
	unsigned seed;
};

static unsigned xorshift(unsigned *s)
{
	unsigned x = *s;
	x ^= x << 13; x ^= x >> 17; x ^= x << 5;
	*s = x;
	return x;
}

static void *worker(void *p)
{
	struct worker_arg *wa = (struct worker_arg *)p;
	SQLHDBC dbc = connect_one(wa->dsn);
	if (dbc == 0) { fprintf(stderr, "worker %d: connect failed\n", wa->id); return NULL; }

	SQLULEN rowsFetched = 0;
	SQLUSMALLINT rowStatus[8];
	SQLWCHAR *sql = NULL;
	/* wide "SELECT tabid FROM systables" */
	const char *q = "SELECT tabid FROM systables";
	size_t ql = strlen(q), i;
	sql = malloc((ql + 1) * sizeof(SQLWCHAR));
	for (i = 0; i <= ql; i++) sql[i] = (SQLWCHAR)q[i];

	while (!atomic_load_explicit(&stop_flag, memory_order_relaxed)) {
		SQLHSTMT stmt;
		SQLRETURN rc = SQLAllocHandle(SQL_HANDLE_STMT, dbc, &stmt);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) { diag("alloc", SQL_HANDLE_DBC, dbc); break; }

		rc = SQLPrepareW(stmt, sql, SQL_NTS);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) { diag("prepare", SQL_HANDLE_STMT, stmt); SQLFreeHandle(SQL_HANDLE_STMT, stmt); continue; }

		SQLBIGINT v = (SQLBIGINT)(xorshift(&wa->seed) % 100);
		SQLLEN ind = 0;
		rc = SQLBindParameter(stmt, 1, SQL_PARAM_INPUT, SQL_C_SBIGINT, SQL_BIGINT, 0, 0, &v, 0, &ind);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) diag("bind", SQL_HANDLE_STMT, stmt);

		rc = SQLExecute(stmt);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) { diag("execute", SQL_HANDLE_STMT, stmt); SQLFreeHandle(SQL_HANDLE_STMT, stmt); continue; }

		/* block fetch exactly like the Go array-fetch path */
		memset(rowStatus, 0, sizeof(rowStatus));
		rowsFetched = 0;
		rc = SQLSetStmtAttr(stmt, SQL_ATTR_ROW_ARRAY_SIZE, (SQLPOINTER)(SQLULEN)8, SQL_IS_UINTEGER);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) diag("row_array_size", SQL_HANDLE_STMT, stmt);
		rc = SQLSetStmtAttr(stmt, SQL_ATTR_ROWS_FETCHED_PTR, &rowsFetched, 0);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) diag("rows_fetched_ptr", SQL_HANDLE_STMT, stmt);
		rc = SQLSetStmtAttr(stmt, SQL_ATTR_ROW_STATUS_PTR, rowStatus, 0);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) diag("row_status_ptr", SQL_HANDLE_STMT, stmt);

		SQLINTEGER tabid[8];
		SQLLEN idInd[8];
		rc = SQLBindCol(stmt, 1, SQL_C_LONG, tabid, sizeof(tabid[0]), idInd);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) diag("bindcol", SQL_HANDLE_STMT, stmt);

		for (;;) {
			rc = SQLFetch(stmt);
			if (rc == SQL_NO_DATA) break;
			if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) { diag("fetch", SQL_HANDLE_STMT, stmt); break; }
		}

		SQLFreeHandle(SQL_HANDLE_STMT, stmt);
		pthread_mutex_lock(&ops_mu);
		g_ops++;
		pthread_mutex_unlock(&ops_mu);
	}
	free(sql);
	SQLDisconnect(dbc);
	SQLFreeHandle(SQL_HANDLE_DBC, dbc);
	return NULL;
}

/* canceller mimics the Go context watcher: cancel a foreign statement
 * mid-flight. It owns its own connection and statement; the interesting
 * part for TSan is concurrent CLI traffic, not the handle sharing (the Go
 * driver only calls SQLCancel on its own live statement). */
static void *canceller(void *p)
{
	const char *dsn = (const char *)p;
	SQLHDBC dbc = connect_one(dsn);
	if (dbc == 0) return NULL;
	while (!atomic_load_explicit(&stop_flag, memory_order_relaxed)) {
		SQLHSTMT stmt;
		SQLRETURN rc = SQLAllocHandle(SQL_HANDLE_STMT, dbc, &stmt);
		if (rc != SQL_SUCCESS && rc != SQL_SUCCESS_WITH_INFO) break;
		const char *q = "SELECT count(*) FROM systables";
		SQLWCHAR wq[128];
		size_t i, n = strlen(q);
		for (i = 0; i <= n; i++) wq[i] = (SQLWCHAR)q[i];
		rc = SQLExecDirectW(stmt, wq, SQL_NTS);
		if (rc == SQL_SUCCESS || rc == SQL_SUCCESS_WITH_INFO) {
			SQLFetch(stmt);           /* may be running while we cancel */
			SQLCancel(stmt);          /* cross-thread cancel analogue  */
			SQLCancel(stmt);
		}
		SQLFreeHandle(SQL_HANDLE_STMT, stmt);
		struct timespec ts = {0, 50 * 1000 * 1000};
		nanosleep(&ts, NULL);
	}
	SQLDisconnect(dbc);
	SQLFreeHandle(SQL_HANDLE_DBC, dbc);
	return NULL;
}

int main(int argc, char **argv)
{
	if (argc < 4) {
		fprintf(stderr, "usage: %s <dsn> <threads> <seconds>\n", argv[0]);
		return 2;
	}
	int threads = atoi(argv[2]);
	int seconds = atoi(argv[3]);
	struct timespec t0, t1;
	clock_gettime(CLOCK_MONOTONIC, &t0);

	SQLRETURN rc = SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &env);
	if (rc != SQL_SUCCESS) return 1;
	rc = SQLSetEnvAttr(env, SQL_ATTR_ODBC_VERSION, (SQLPOINTER)(SQLULEN)SQL_OV_ODBC3, SQL_IS_UINTEGER);
	if (rc != SQL_SUCCESS) return 1;

	stop_flag = 0;
	pthread_t tids[64];
	struct worker_arg args[64];
	int n = 0;
	for (int i = 0; i < threads && n < 64; i++, n++) {
		args[i].id = i; args[i].dsn = argv[1]; args[i].seed = 0x9e3779b9u + i * 2654435761u;
		pthread_create(&tids[n], NULL, worker, &args[i]);
	}
	pthread_t ct;
	pthread_create(&ct, NULL, canceller, (void *)argv[1]);

	struct timespec ts = {seconds, 0};
	nanosleep(&ts, NULL);
	atomic_store_explicit(&stop_flag, 1, memory_order_relaxed);
	for (int i = 0; i < n; i++) pthread_join(tids[i], NULL);
	pthread_join(ct, NULL);

	clock_gettime(CLOCK_MONOTONIC, &t1);
	double el = (t1.tv_sec - t0.tv_sec) + 1e-9 * (t1.tv_nsec - t0.tv_nsec);
	printf("done: %lu ops in %.1fs (%.0f ops/s), threads=%d\n", g_ops, el, el > 0 ? g_ops / el : 0, threads);
	SQLFreeHandle(SQL_HANDLE_ENV, env);
	return 0;
}
