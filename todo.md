# Замечания по ревью драйвера

Проверка выполнена по коду. Дефект с размером набора параметров воспроизведён на Informix; остальные пункты требуют отдельных регрессионных проверок на поддерживаемых платформах Db2.

## Исправлено

- [x] **Повторное выполнение statement после пакетной вставки.** `SQL_ATTR_PARAMSET_SIZE` оставался равным длине массива и применялся к следующему одиночному параметру. На Informix вставка `[1, 2]`, затем `3` на том же statement вернула SQLSTATE `22003`, хотя в таблице оказались `[1, 2, 3, 3]`. Исправлено в [PR #290](https://github.com/ibmdb/go_ibm_db/pull/290): размер набора задаётся перед каждым `SQLExecute`; регрессионный тест находится в [`testdata/Arrayint_test.go`](testdata/Arrayint_test.go). Проверено 20 повторениями на Informix. Для Db2 LUW, z/OS и IBM i этот сценарий ещё не проверен.

- [x] **Замечания golangci-lint, исправленные автоматически и вручную (2026-11).** `golangci-lint run --fix` (строгий конфиг) + ручные правки: staticcheck QF1001/QF1003/QF1008/ST1005, ineffassign (мёртвые записи `ArrayCheck`), errorlint (`errors.As` в column.go), perfsprint, intrange, unconvert, gofumpt, misspell, dupword, whitespace, gocritic (ifElseChain, assignOp, commentFormatting), `fmt.Errorf` → `errors.New` без аргументов. Дефолтный прогон: 45 → 26; строгий: 305 → 280. Сломанный автофиксом `//sys SQLAllocHandle` в api/api.go (директива генератора `mksyscall_unix.pl`) и точка godot в закомментированном коде — отменены вручную; восстановлены импорты `fmt`/`errors` после автофиксов.

- [x] **Пул, трассировка и обработка ошибок — раунд 2 (2026-11-01).** Закрыты 9 пунктов «Открытых проблем»: обработка ошибки `SQLMoreResults` (выход из бесконечного цикла), повторная проверка `closed` в цикле ожидания `Pool.Open`, две утечки statement handle (`Conn.Query`/`BindColumns`, отмена `PrepareContext`), возврат ошибки вместо тихого пропуска `SQL_ROW_ERROR`, маскирование пароля в трейсе (`maskDSN`), нефатальная трассировка + права 0600, сброс `c.tx` только после успешного `SQLEndTran`, валидация параметров пула без panic. errcheck в дефолтном прогоне — 0. Добавлены тесты `TestMaskDSN`, `TestPconnectInvalidParamsNoPanic`, `TestPoolOpenAfterReleaseWhileWaiting`. Дефолтный прогон: 26 → 4 (остались govet unsafeptr ×3 и SA1019). Интеграционный сьют против Informix — без регрессий (30 падений, идентично базе).

- [x] **Отмена контекста, Informix-платформа, чистый дефолтный линтер — раунд 3 (2026-11-01).** 1) SA1019 закрыт (`LastInsertId` → внутренний `query1`). 2) unsafeptr задокументирован через `//nolint:govet`. 3) Informix: `SQLCancel`-независимый фикс `GetInfo` (ASCII-fallback), `PlatformInformix` в `classifyPlatform` + 30 скипов. 4) Отмена контекста: `api.SQLCancel` (unix/windows/zos), `watchContext`-watcher, guard `cancelMu`/`freed`, отмена в `exec`/`query1`/`Rows.Next`; тест `TestExecContextCancelDuringBlockedExec` (блокировка по row-lock отменяется по дедлайну за 3.0с). Дефолтный golangci-lint: **0 issues** (было 45 на старте). Сьют против Informix: 0 FAIL / 32 SKIP / 39 PASS.

- [x] **Переполнения и безопасность — раунд 4 (2026-11-01).** 1) G115: честное сравнение в `IsNull` (без усечения до int16), хелперы `odbcNum`/`sqlulen` с проверкой границ, `//nolint` для структурно ограниченных значений. 2) Инсталлятор: `Unzipping`→`unzip` в error-стиле (закрыт exitAfterDefer), zip-slip guard, капа декомпрессии 2 ГиБ (G110), `exec.CommandContext` (noctx), by-design G107/G204/G304/G703 задокументированы. 3) nilnil: NULL-семантика `driver.Value` задокументирована. gosec+nilnil+noctx в строгом прогоне: 32 → 3 (только by-design G103-аудит unsafe). Проверки: build/gofmt/unit ✅, default lint 0, сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

## Открытые проблемы

- [x] **Высокий: отмена контекста не останавливает блокирующие операции ODBC.** *Исправлено 2026-11-01:* добавлен `api.SQLCancel` (все платформы) и `watchContext` — watcher-горутина вызывает `SQLCancel` при `ctx.Done()` для блокирующих `SQLExecute` (`Stmt.exec`, `Stmt.query1`) и `SQLFetch` (`Rows.Next`, контекст хранится в `Rows`). Гонка с освобождением handle закрыта через `cancelMu`/`freed` в `ODBCStmt`. Интеграционный тест `TestExecContextCancelDuringBlockedExec`: заблокированный на row-lock UPDATE отменяется ровно по дедлайну (3.0с) и возвращает `context deadline exceeded`.

- [x] **Высокий: ошибка `SQLMoreResults` игнорируется при `Exec`.** *Исправлено 2026-11-01:* при ошибке `SQLMoreResults` цикл теперь возвращает `NewError("SQLMoreResults", …)` вместо бесконечного повторения; выход — только по `SQL_NO_DATA`.

- [x] **Высокий: `Pool.Open` может вызвать panic после `Pool.Release`.** *Исправлено 2026-11-01:* `closed` проверяется в каждой итерации цикла ожидания под mutex. Покрыто тестом `TestPoolOpenAfterReleaseWhileWaiting` (Release во время ожидания Open).

- [x] **Высокий: утечка statement handle при ошибке `BindColumns` в `Conn.Query`.** *Исправлено 2026-11-01:* все ветки ошибки после выделения handle вызывают `releaseHandle`.

- [x] **Высокий: утечка statement handle при отмене `PrepareContext`.** *Исправлено 2026-11-01:* при отменённом контексте после успешной подготовки вызывается `os.closeByStmt()`.

- [x] **Высокий: пакетное чтение молча пропускает строки с `SQL_ROW_ERROR`.** *Исправлено 2026-11-01:* `SQL_ROW_ERROR` и прочие аномальные статусы возвращают `NewError("SQLFetch", …)`; пропускается только `SQL_ROW_NOROW`.

- [x] **Высокий: трассировка раскрывает пароль из DSN.** *Исправлено 2026-11-01:* добавлена `maskDSN` (регистронезависимо маскирует `PWD=***`); применяется в `Driver.Open`, `Pool.Open`, `Pool.Init`, `CreateDb`/`DropDb`. Тест — `TestMaskDSN`.

- [x] **Высокий: ошибка открытия файла трассировки завершает процесс.** *Исправлено 2026-11-01:* `Trace1` не вызывает `log.Fatal` — печатает предупреждение в stderr (однократно) и продолжает работу; права файла снижены до 0600 (закрывает и gosec G302).

- [x] **Требует проверки: состояние транзакции сбрасывается до успешного `SQLEndTran`.** *Исправлено 2026-11-01:* `c.tx = nil` теперь выполняется только после успешного `SQLEndTran`; при ошибке транзакция остаётся активной (повторный `Commit`/`Rollback` возможен, новый `Begin` честно вернёт «already in a transaction»). Поведение при ошибке commit на реальной БД ещё не проверено.

- [x] **Низкий: некорректные параметры пула могут вызвать panic.** *Исправлено 2026-11-01:* `Pconnect`/`Pool.Open` используют `strings.SplitN(…, 2)` с проверкой длины и обрабатывают ошибку `strconv.Atoi` (невалидное значение откатывается к значению по умолчанию). Тест — `TestPconnectInvalidParamsNoPanic`.

## Совместимость с Informix

Прогон 2026-11-01 против Informix Developer Database в контейнере (`sendsay-ifx-probe`), подключение через DRDA-порт `informix_dr` (9089 → `127.0.0.1:19089`), DSN: `DATABASE=sysmaster;HOSTNAME=127.0.0.1;PORT=19089;PROTOCOL=TCPIP;UID=informix;PWD=…`.

**Вывод: полной совместимости с Informix нет — падает 30 из 70 тестов (38 pass, 2 skip). Падения существуют и без текущих правок линтера (проверено сравнением с базой через `git stash`: списки упавших тестов идентичны), то есть это не регрессия, а систематическая несовместимость.**

*Обновление 2026-11-01 (вечер):* реализован вариант «минимум» — Informix распознаётся как платформа, несовместимые тесты корректно скипаются. Итог прогона против Informix: **0 FAIL / 32 SKIP / 39 PASS.**

Группы падений:

- [ ] **Высокий: вставка массивов параметров не работает — 16 тестов.** `TestBigintArray`, `TestBoolArray`, `TestCharacterArray`, `TestVarcharArray`, `TestSmallintArray`, `TestInt2Array`, `TestDoubleArray`, `TestFloatArray`, `TestRealArray`-класс, `TestDec2Array`, `TestDecimal2Array`, `TestNumeric2Array`, `TestTimeStampArray`, `TestClobArray`, `TestClob2Array`, `TestGraphicArray`, `TestVargraphicArray`. Пример: `[]int2` → `22003 Value exceeds limit of INTEGER precision` (массив привязывается как BIGINT без учёта типа колонки), `[]bool` — ошибка вставки. При этом базовый сценарий пакетной вставки из [PR #290](https://github.com/ibmdb/go_ibm_db/pull/290) (`Arrayint_test.go`) проходит — не работает именно типизированная привязка.
- [ ] **Высокий: типы, отсутствующие в Informix — 2+ теста.** `decfloat` (`TestDecfloat`), `dbclob` (`TestDBClobDoubleByteChars`) → `IX000 Type … not found`.
- [ ] **Средний: DB2-специфичный SQL в тестах/драйвере — 7 тестов.** `sysibm.sysdummy1` + `identity_val_local()` в `LastInsertId` → `42000`; null-* тесты (`Bool`, `Character`, `Float`, `Integer`, `String`, `Time`), `TestDecimalInsertByString` → `42000 A syntax error has occurred`.
- [ ] **Средний: транзакции.** `TestRollbackTransaction` падает на Informix.
- [ ] **Прочее.** `TestAllDataTypes`, `TestChineseChar`, `TestCodeunits32` (unicode/graphic-типы).

Что делать (варианты):

- [x] **Минимум:** не декларировать Informix как поддерживаемую платформу. *Сделано 2026-11-01:* `classifyPlatform` распознаёт `IDS/…` → `PlatformInformix` (для этого же починен `GetInfo`: Informix DRDA возвращает `SQL_DBMS_NAME` однобайтовой ASCII через wide-API — добавлен fallback-декод); 30 несовместимых тестов помечены `SkipOnPlatform(t, PlatformInformix)`. Сьют на Informix: 0 FAIL / 32 SKIP / 39 PASS.
- [ ] **Максимум:** чинить совместимость — типизированная привязка массивов (учёт типа колонки при `SQLBindParameter`), замена `sysibm.sysdummy1`/`identity_val_local()` на переносимые альтернативы, поддержка типов. Объём большой, нужен отдельный план.



Выполнены два прогона (требуются заголовки clidriver: `IBM_DB_HOME`, `CGO_CFLAGS`, `CGO_LDFLAGS`, `LD_LIBRARY_PATH`):

- **Дефолтный конфиг — 45 замечаний:** errcheck 22, staticcheck 17, govet 3, ineffassign 3.
- **Строгий конфиг — 305 замечаний** (45+ линтеров, без исключений: gosec, gocritic, err113, revive, mnd, lll, funlen, gocognit, nestif, gofumpt и др.): revive 50, lll 43, err113 42, mnd 33, errcheck 30, gosec 28, staticcheck 13, nestif 8, gocritic 8, paralleltest 7, gocognit 6, exhaustive 4, по 3 — ineffassign, intrange, nilnil, perfsprint, gofumpt, godox, godot, gochecknoglobals, по 1 — govet, whitespace, noctx, misspell, gochecknoinits, funlen, errorlint, dupword.

Полные отчёты: `/tmp/opencode/lint_default.txt`, `/tmp/opencode/lint_hard.txt`; строгий конфиг — `/tmp/opencode/golangci.hard.yml` (можно положить в репо как `.golangci.yml`). Замечания на одинаковых `file:line:col` дедуплицируются, поэтому часть govet/staticcheck-точек (например, `odbcstmt.go:212`, `:256`) видна только в дефолтном прогоне. `db2dialect/` — отдельный модуль и из корня не проверяется.

## Замечания golangci-lint 2.14.0

Выполнены два прогона (требуются заголовки clidriver: `IBM_DB_HOME`, `CGO_CFLAGS`, `CGO_LDFLAGS`, `LD_LIBRARY_PATH`):

### Безопасность и корректность — приоритет

- [x] **Средний: конвертации целых без проверки переполнения (gosec G115, 13 шт.).** *Исправлено 2026-11-01:* `IsNull` сравнивает на полной ширине SQLLEN (реальный баг: усечение до int16 могло дать ложный NULL); добавлены `odbcNum` (0-based индекс → 1-based номер, panic при переполнении) и `sqlulen` (len → SQLULEN с ошибкой) для строковых/бинарных параметров; структурно ограниченные поля (календарь, фиксированные буферы) помечены `//nolint:gosec` с обоснованием. Остались только 3×G103 (аудит unsafe — by design для ODBC-буферов).
- [x] **Средний: инсталлятор — несколько классов уязвимостей (gosec).** *Исправлено 2026-11-01:* `Unzipping` переписан в error-стиле — `os.Exit` больше не обходит `defer` (exitAfterDefer), добавлен zip-slip guard (запись только внутри целевого каталога) и капа декомпрессии 2 ГиБ/файл (G110); `exec.Command` → `exec.CommandContext` (noctx); by-design пункты (G107 URL из env, G204/G305 фиксированные tar/gunzip-команды, G304/G703 служебные пути) помечены `//nolint:gosec` с обоснованием. Устаревший инсталлятор прогоняется только в CI — риск ограничен.
- [x] **Средний: `os.Exit` до `defer` (gocritic exitAfterDefer).** *Исправлено 2026-11-01:* `Unzipping` переписан (см. пункт про инсталлятор): ошибки возвращаются, `os.Exit` — только в обёртке после освобождения ресурсов.
- [x] **Средний: возврат nil-ошибки с невалидным значением (nilnil).** *Закрыто 2026-11-01:* во всех 5 местах `return nil, nil` — это корректная кодировка SQL NULL для `driver.Value` (nil — валидное значение); помечено `//nolint:nilnil` с обоснованием.
- [x] **Низкий: `exec.Command` без контекста (noctx).** *Исправлено 2026-11-01:* переведён на `exec.CommandContext`.
- [x] **Средний: возможное некорректное использование `unsafe.Pointer` (govet unsafeptr / G103).** *Закрыто 2026-11-01:* три конвертации `uintptr→SQLPOINTER` в `odbcstmt.go` — легальный паттерн ODBC (integer-атрибуты передаются через pointer-аргумент), помечены `//nolint:govet` с обоснованием; `api/zapi_unix.go` — сгенерированный cgo-мост, аналогично помечен (nolint потеряется при регенерации `mksyscall_unix.pl` — учесть). G103 (аудит unsafe) остаётся осознанным — код сплошь работает с ODBC-буферами.
- [x] **Средний: type assertion на error.** `sqlOut.go`: 452 — не срабатывает на обёрнутых ошибках. *Исправлено: заменено на `errors.As`.*
- [x] **Низкий: права 0666 на файл трассировки (gosec G302).** *Исправлено 2026-11-01:* права снижены до 0600; заодно сбой открытия файла больше не фатален (см. «Открытые проблемы»).

### Потерянные ошибки и dead code

- [x] **Средний: игнорируемые ошибки (errcheck, 30 шт.).** *Исправлено 2026-11-01:* все вызовы в пакете драйвера и инсталляторе либо проверяют ошибку, либо явно отбрасывают (`_ =`); дефолтный errcheck-набор = 0.
- [x] **Средний: мёртвые записи (ineffassign).** `odbcstmt.go`: 171, 179, 187 — `ArrayCheck = 0` никогда не читается. *Исправлено: все 5 присваиваний удалены.*
- [x] **Средний: вызов устаревшего API (SA1019).** *Исправлено 2026-11-01:* `LastInsertId` зовёт внутренний `query1(context.Background(), nil)` вместо deprecated `driver.Stmt.Query`.

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
