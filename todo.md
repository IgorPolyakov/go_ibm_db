# Замечания по ревью драйвера

Проверка выполнена по коду. Дефект с размером набора параметров воспроизведён на Informix; остальные пункты требуют отдельных регрессионных проверок на поддерживаемых платформах Db2.

## Исправлено

- [x] **Повторное выполнение statement после пакетной вставки.** `SQL_ATTR_PARAMSET_SIZE` оставался равным длине массива и применялся к следующему одиночному параметру. На Informix вставка `[1, 2]`, затем `3` на том же statement вернула SQLSTATE `22003`, хотя в таблице оказались `[1, 2, 3, 3]`. Исправлено в [PR #290](https://github.com/ibmdb/go_ibm_db/pull/290): размер набора задаётся перед каждым `SQLExecute`; регрессионный тест находится в [`testdata/Arrayint_test.go`](testdata/Arrayint_test.go). Проверено 20 повторениями на Informix. Для Db2 LUW, z/OS и IBM i этот сценарий ещё не проверен.

- [x] **Замечания golangci-lint, исправленные автоматически и вручную (2026-11).** `golangci-lint run --fix` (строгий конфиг) + ручные правки: staticcheck QF1001/QF1003/QF1008/ST1005, ineffassign (мёртвые записи `ArrayCheck`), errorlint (`errors.As` в column.go), perfsprint, intrange, unconvert, gofumpt, misspell, dupword, whitespace, gocritic (ifElseChain, assignOp, commentFormatting), `fmt.Errorf` → `errors.New` без аргументов. Дефолтный прогон: 45 → 26; строгий: 305 → 280. Сломанный автофиксом `//sys SQLAllocHandle` в api/api.go (директива генератора `mksyscall_unix.pl`) и точка godot в закомментированном коде — отменены вручную; восстановлены импорты `fmt`/`errors` после автофиксов.

## Открытые проблемы

- [ ] **Высокий: отмена контекста не останавливает блокирующие операции ODBC.** `ExecContext` и `QueryContext` проверяют `ctx.Done()` только после `SQLExecute`, а `Rows.Next` вызывает `SQLFetch` без контекста. Тайм-аут Go не прерывает зависший вызов драйвера. Проверить отмену при выполнении запроса и чтении строк, реализовать её с учётом потокобезопасности ODBC. См. [`stmt.go`](stmt.go#L111) и [`rows.go`](rows.go#L186).

- [ ] **Высокий: ошибка `SQLMoreResults` игнорируется при `Exec`.** Цикл завершается только на `SQL_NO_DATA`; при `SQL_ERROR` он продолжает работу и может повторять ошибку бесконечно. Возвращать диагностику `SQLMoreResults` и добавить сценарий ошибки нескольких результатов. См. [`stmt.go`](stmt.go#L115).

- [ ] **Высокий: `Pool.Open` может вызвать panic после `Pool.Release`.** `closed` проверяется перед циклом ожидания, но не после повторного захвата mutex. `Release` обнуляет карты; ожидающий `Open` может затем записать в `nil`-карту. Проверять `closed` внутри каждой итерации под mutex и покрыть конкурентным тестом. См. [`pooling.go`](pooling.go#L122) и [`pooling.go`](pooling.go#L135).

- [ ] **Высокий: утечка statement handle при ошибке `BindColumns` в `Conn.Query`.** После выделения handle ветка ошибки возвращается без `releaseHandle`. Освобождать handle на всех путях ошибки и проверять счётчик handles. См. [`conn.go`](conn.go#L143) и [`conn.go`](conn.go#L175).

- [ ] **Высокий: утечка statement handle при отмене `PrepareContext`.** Подготовка уже выделила handle, но при отменённом контексте метод возвращает `ctx.Err()` без закрытия statement. Закрывать его перед возвратом и проверить отменённый контекст тестом. См. [`stmt.go`](stmt.go#L38).

- [ ] **Высокий: пакетное чтение молча пропускает строки с `SQL_ROW_ERROR`.** В `Rows.Next` все статусы кроме `SQL_ROW_SUCCESS` и `SQL_ROW_SUCCESS_WITH_INFO` пропускаются, поэтому вызывающий код может получить неполный результат без ошибки. Для `SQL_ROW_ERROR` возвращать диагностику; пропускать только `SQL_ROW_NOROW`. См. [`rows.go`](rows.go#L169).

- [ ] **Высокий: трассировка раскрывает пароль из DSN.** При включённом `GO_IBMDB_TRACE` строка подключения целиком записывается в лог. Удалять или маскировать `PWD` до логирования; проверить также трассировку в пуле. См. [`conn.go`](conn.go#L56) и [`pooling.go`](pooling.go#L100).

- [ ] **Высокий: ошибка открытия файла трассировки завершает процесс.** `Trace1` вызывает `log.Fatal`, если `os.OpenFile` вернул ошибку. Сделать сбой трассировки нефатальным для приложения. См. [`log2/logger1.go`](log2/logger1.go#L44).

- [ ] **Требует проверки: состояние транзакции сбрасывается до успешного `SQLEndTran`.** `c.tx = nil` выполняется до commit/rollback; при ошибке ODBC объект соединения теряет состояние текущей транзакции. Определить корректное восстановление состояния и проверить ошибки commit/rollback. См. [`tx.go`](tx.go#L39).

- [ ] **Низкий: некорректные параметры пула могут вызвать panic.** `Pconnect("PoolSize")` и `Pool.Open(..., "SetConnMaxLifetime")` обращаются к `opt[1]` без проверки результата `strings.Split`. Обрабатывать неверный формат без panic с учётом текущего API пула. См. [`pooling.go`](pooling.go#L43) и [`pooling.go`](pooling.go#L109).

## Замечания golangci-lint 2.14.0

Выполнены два прогона (требуются заголовки clidriver: `IBM_DB_HOME`, `CGO_CFLAGS`, `CGO_LDFLAGS`, `LD_LIBRARY_PATH`):

- **Дефолтный конфиг — 45 замечаний:** errcheck 22, staticcheck 17, govet 3, ineffassign 3.
- **Строгий конфиг — 305 замечаний** (45+ линтеров, без исключений: gosec, gocritic, err113, revive, mnd, lll, funlen, gocognit, nestif, gofumpt и др.): revive 50, lll 43, err113 42, mnd 33, errcheck 30, gosec 28, staticcheck 13, nestif 8, gocritic 8, paralleltest 7, gocognit 6, exhaustive 4, по 3 — ineffassign, intrange, nilnil, perfsprint, gofumpt, godox, godot, gochecknoglobals, по 1 — govet, whitespace, noctx, misspell, gochecknoinits, funlen, errorlint, dupword.

Полные отчёты: `/tmp/opencode/lint_default.txt`, `/tmp/opencode/lint_hard.txt`; строгий конфиг — `/tmp/opencode/golangci.hard.yml` (можно положить в репо как `.golangci.yml`). Замечания на одинаковых `file:line:col` дедуплицируются, поэтому часть govet/staticcheck-точек (например, `odbcstmt.go:212`, `:256`) видна только в дефолтном прогоне. `db2dialect/` — отдельный модуль и из корня не проверяется.

### Безопасность и корректность — приоритет

- [ ] **Средний: конвертации целых без проверки переполнения (gosec G115, 13 шт.).** `int→int16/uint16/int32/uint32` в [`column.go`](column.go#L33): 33, 39, 48, 49, 53, 54, 74, 370; [`param.go`](param.go#L76): 76, 127, 150; [`error.go`](error.go#L71): 71, 76. Добавить проверки границ или безопасные конвертации.
- [ ] **Средний: инсталлятор — несколько классов уязвимостей (gosec).** [`installer/setup.go`](installer/setup.go#L96): G204 — подпроцессы с потенциально заражённым входом (96, 121, 132); G703 — path traversal (22, 27, 261); G305 — file traversal при распаковке архива (74); G110 — декомпрессионная бомба (85); G107 — HTTP-запрос по переменному URL (33); G304 — включение файла по переменной (45, 79). Оценить риски: код выполняется в CI через `go run installer/setup.go`.
- [ ] **Средний: `os.Exit` до `defer` (gocritic exitAfterDefer).** [`installer/setup.go`](installer/setup.go#L71) — отложенный `reader.Close()` не выполнится; проверить остальные `os.Exit` инсталлятора.
- [ ] **Средний: возможное некорректное использование `unsafe.Pointer` (govet unsafeptr / G103).** [`api/zapi_unix.go`](api/zapi_unix.go#L106); `odbcstmt.go`: 212, 256 (видны в дефолтном прогоне); аудит unsafe: `api/api.go`: 111, 124; `column.go`: 40.
- [ ] **Средний: возврат nil-ошибки с невалидным значением (nilnil).** [`column.go`](column.go#L215): 215, 221, 347 — использовать sentinel-ошибку.
- [x] **Средний: type assertion на error.** `sqlOut.go`: 452 — не срабатывает на обёрнутых ошибках. *Исправлено: заменено на `errors.As`.*
- [ ] **Низкий: `exec.Command` без контекста (noctx).** [`installer/setup.go`](installer/setup.go#L203).
- [ ] **Низкий: права 0666 на файл трассировки (gosec G302).** [`log2/logger1.go`](log2/logger1.go#L45) — ожидается ≤0600. Связано с открытым пунктом про `log.Fatal` в `Trace1`.

### Потерянные ошибки и dead code

- [ ] **Средний: игнорируемые ошибки (errcheck, 30 шт.).** По группам:
  - `pooling.go` — `Close()` ×7 (199, 217, 246, 249, 285, 305, 313) и `strconv.Atoi` ×2 (45, 111 — усугубляет открытый пункт про неверные параметры пула);
  - `installer/setup.go` ×12 — 37, 49, 66, 73, 76, 84 (`Close`/`MkdirAll`), 111–113 (`Output`), 245, 246, 251 (`filepath.Abs`);
  - `log2/logger1.go` — 49 (`Close`), 54 (`Sync`); `conn.go` — 81, 161; `column.go` — 438; `result.go` — 30, 36 (`Close`); `stmt.go` — 103, 166 (`closeByStmt`).
- [x] **Средний: мёртвые записи (ineffassign).** `odbcstmt.go`: 171, 179, 187 — `ArrayCheck = 0` никогда не читается. *Исправлено: все 5 присваиваний удалены.*
- [ ] **Средний: вызов устаревшего API (SA1019).** [`result.go`](result.go#L32) — `driver.Stmt.Query` deprecated; реализовать `StmtQueryContext` (пересекается с пунктом про отмену контекста).

### Сложность — кандидат на рефакторинг

- [ ] **Высокая когнитивная сложность (gocognit, порог 30).** [`installer/setup.go`](installer/setup.go#L190) `main` — 89; [`sqlOut.go`](sqlOut.go#L248) `ConvertAssign` — 71; [`param.go`](param.go#L42) `BindValue` — 70; `odbcstmt.go`: 122 (`Exec` — 45), 237 (`BindColumns` — 31); [`rows.go`](rows.go#L146) `Next` — 34.
- [ ] **Слишком длинные функции (funlen).** [`sqlOut.go`](sqlOut.go#L33) `newOut` — 75 операторов (>60).
- [ ] **Глубокая вложенность (nestif, 8 шт.).** `installer/setup.go`: 172, 204, 382; `odbcstmt.go`: 146, 288; `pooling.go`: 223; `rows.go`: 150; `sqlOut.go`: 42.
- [ ] **Неполные switch по `reflect.Kind` (exhaustive).** [`sqlOut.go`](sqlOut.go#L349): 349, 405, 479, 497 — отсутствующие ветки не покрыты `default`.

### Стиль и гигиена — низкий приоритет

- [ ] **Динамические ошибки вместо sentinel/`%w` (err113, 42 шт.).** `column.go`: 96, 149, 255, 314, 350, 385, 392; `database.go`: 21, 34, 97; `handle.go`: 31, 43; `installer/setup.go`: 41; `odbcstmt.go`: 128, 172, 180, 188, 250; `param.go`: 298, 376; `result.go`: 63; `sqlOut.go` ×15 (47, 80, 130, 219, 227, 231, 236, 256, 262, 268, 380, 411, 420, 429, 445); `stats.go`: 37; `stmt.go`: 66, 97, 161; `tx.go`: 29, 41.
- [x] **staticcheck QF/ST (9 шт., помимо SA1019).** *Исправлено:* QF1001 — `error.go`; ST1005 — `installer/setup.go`: 35, 41, 47, 54; QF1008 — `conn.go`, `driver.go`, `pooling.go`, `installer/setup.go`; QF1003 — `installer/setup.go` (переписаны на switch).
- [x] **gocritic.** *Исправлено:* ifElseChain → switch (`installer/setup.go`: 209, 271, 281, 390; `database.go`: 72); assignOp (`column.go`: 306); commentFormatting (`log2/logger1.go`). Остаётся только exitAfterDefer — см. блок безопасности.
- [ ] **revive (50 шт.).** Преимущественно отсутствие комментариев у exported-символов `api/` (типы и функции `SQL*`, `Trace1`, `GetPath`, `Extract`, константы `SQL_SS_XML`, `SQL_DRIVER_NAME`, `SQL_ATTR_CONNECTION_POOLING` — ≈44); var-naming ALL_CAPS ×3; package-comments для `log2` ×2; indent-error-flow ×1.
- [ ] **Строки длиннее 120 символов (lll, 43 шт.).** `api/api.go` ×12 (52–71); `api/api_unix.go` ×4 (23, 27, 206, 217); `api/zapi_unix.go` ×20 (42–310); `installer/setup.go`: 210, 212, 214, 216, 387; `column.go`: 68; `param.go`: 327.
- [ ] **Магические числа (mnd, 33 шт.).** `column.go` ×8 (47, 84, 109, 111, 113, 147, 233, 424); `conn.go`: 36, 37, 101; `error.go`: 64; `installer/setup.go` ×9 (248, 258, 274, 283, 288, 345, 363, 366, 373); `odbcstmt.go`: 142; `param.go`: 139, 295; `pooling.go`: 166; `sqlOut.go` ×6 (77, 120, 499, 501, 503, 505); `utf16.go`: 33, 68.
- [ ] **godot (3 шт.).** Комментарии без точки: `api/api.go`: 89; `api/api_unix.go`: 29; `conn.go`: 133.
- [ ] **godox (3 шт.).** TODO-маркеры: `driver.go`: 52; `odbcstmt.go`: 20; `param.go`: 43 — либо закрыть, либо завести задачи.
- [x] **intrange.** *Исправлено:* `database.go`: 26; `param.go` (все циклы по `d`); `pooling_test.go`: 52.
- [x] **gofumpt.** *Исправлено:* `api/zapi_unix.go`: 257; `column.go`: 203; `handle.go`: 8. CI по-прежнему проверяет только `gofmt`.
- [x] **perfsprint.** *Исправлено:* `fmt.Sprintf` → конкатенация: `api/api.go`: 92; `conn.go`: 58, 136 (неиспользуемый импорт `fmt` удалён).
- [x] **Опечатки.** *Исправлено:* `stmt.go`: 75 — «the the»; `stmt.go`: 193 — «implementes» → «implements».
- [x] **whitespace.** *Исправлено:* `param.go`: 40 — лишняя пустая строка удалена.
- [ ] **Глобальные состояния (gochecknoglobals/gochecknoinits).** `driver.go`: 17 (`drv`), 60 (`init`); `odbcstmt.go`: 120 (`testingIssue5`); `log2/logger1.go`: 14 (`globalvar`) — осознанно принять (`//nolint`) или изолировать.
- [ ] **paralleltest (7 шт.).** Тесты без `t.Parallel()`: `dsn_test.go`: 12, 71; `pooling_test.go`: 10, 23, 44, 77, 98 — учитывать, что тесты `testdata/` требуют живую БД и параллелизм там небезопасен.
