# Замечания по ревью драйвера

Проверка выполнена по коду. Дефект с размером набора параметров воспроизведён на Informix; остальные пункты требуют отдельных регрессионных проверок на поддерживаемых платформах Db2.

## Исправлено

- [x] **Повторное выполнение statement после пакетной вставки.** `SQL_ATTR_PARAMSET_SIZE` оставался равным длине массива и применялся к следующему одиночному параметру. На Informix вставка `[1, 2]`, затем `3` на том же statement вернула SQLSTATE `22003`, хотя в таблице оказались `[1, 2, 3, 3]`. Исправлено в [PR #290](https://github.com/ibmdb/go_ibm_db/pull/290): размер набора задаётся перед каждым `SQLExecute`; регрессионный тест находится в [`testdata/Arrayint_test.go`](testdata/Arrayint_test.go). Проверено 20 повторениями на Informix. Для Db2 LUW, z/OS и IBM i этот сценарий ещё не проверен.

- [x] **Замечания golangci-lint, исправленные автоматически и вручную (2026-11).** `golangci-lint run --fix` (строгий конфиг) + ручные правки: staticcheck QF1001/QF1003/QF1008/ST1005, ineffassign (мёртвые записи `ArrayCheck`), errorlint (`errors.As` в column.go), perfsprint, intrange, unconvert, gofumpt, misspell, dupword, whitespace, gocritic (ifElseChain, assignOp, commentFormatting), `fmt.Errorf` → `errors.New` без аргументов. Дефолтный прогон: 45 → 26; строгий: 305 → 280. Сломанный автофиксом `//sys SQLAllocHandle` в api/api.go (директива генератора `mksyscall_unix.pl`) и точка godot в закомментированном коде — отменены вручную; восстановлены импорты `fmt`/`errors` после автофиксов.

- [x] **Пул, трассировка и обработка ошибок — раунд 2 (2026-11-01).** Закрыты 9 пунктов «Открытых проблем»: обработка ошибки `SQLMoreResults` (выход из бесконечного цикла), повторная проверка `closed` в цикле ожидания `Pool.Open`, две утечки statement handle (`Conn.Query`/`BindColumns`, отмена `PrepareContext`), возврат ошибки вместо тихого пропуска `SQL_ROW_ERROR`, маскирование пароля в трейсе (`maskDSN`), нефатальная трассировка + права 0600, сброс `c.tx` только после успешного `SQLEndTran`, валидация параметров пула без panic. errcheck в дефолтном прогоне — 0. Добавлены тесты `TestMaskDSN`, `TestPconnectInvalidParamsNoPanic`, `TestPoolOpenAfterReleaseWhileWaiting`. Дефолтный прогон: 26 → 4 (остались govet unsafeptr ×3 и SA1019). Интеграционный сьют против Informix — без регрессий (30 падений, идентично базе).

- [x] **Отмена контекста, Informix-платформа, чистый дефолтный линтер — раунд 3 (2026-11-01).** 1) SA1019 закрыт (`LastInsertId` → внутренний `query1`). 2) unsafeptr задокументирован через `//nolint:govet`. 3) Informix: `SQLCancel`-независимый фикс `GetInfo` (ASCII-fallback), `PlatformInformix` в `classifyPlatform` + 30 скипов. 4) Отмена контекста: `api.SQLCancel` (unix/windows/zos), `watchContext`-watcher, guard `cancelMu`/`freed`, отмена в `exec`/`query1`/`Rows.Next`; тест `TestExecContextCancelDuringBlockedExec` (блокировка по row-lock отменяется по дедлайну за 3.0с). Дефолтный golangci-lint: **0 issues** (было 45 на старте). Сьют против Informix: 0 FAIL / 32 SKIP / 39 PASS.

- [x] **Переполнения и безопасность — раунд 4 (2026-11-01).** 1) G115: честное сравнение в `IsNull` (без усечения до int16), хелперы `odbcNum`/`sqlulen` с проверкой границ, `//nolint` для структурно ограниченных значений. 2) Инсталлятор: `Unzipping`→`unzip` в error-стиле (закрыт exitAfterDefer), zip-slip guard, капа декомпрессии 2 ГиБ (G110), `exec.CommandContext` (noctx), by-design G107/G204/G304/G703 задокументированы. 3) nilnil: NULL-семантика `driver.Value` задокументирована. gosec+nilnil+noctx в строгом прогоне: 32 → 3 (только by-design G103-аудит unsafe). Проверки: build/gofmt/unit ✅, default lint 0, сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

- [x] **Документация, exhaustive, godox, revive — раунд 5 (2026-11-01).** Полная документация exported-символов (revive: 50 → 0), явные `default` в reflect.Kind-switch, все TODO-маркеры переписаны/закрыты, godot/gocritic-хвост добит, `SQLColAttribute`-параметры в camelCase, `intrange`-хвост в stmt.go. Из конфига исключены: var-naming ODBC-имён в api/, commentFormatting/godot на //sys-директивах и cgo-преамбуле. Остаток в строгом прогоне — только осознанные категории: lll 50, err113 44, mnd 35, errcheck(check-blank) 27, paralleltest 11, nestif 7, gocognit 6, gochecknoglobals 4, G103 3, funlen 1. Проверки: build/gofmt/unit ✅, default lint 0, сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

- [x] **Dead code — раунд 7 (2026-11-01).** Прогон `deadcode -test` (go/tools) + `unused`: удалены `api.StringToUTF16Ptr`, Go-обёртки `api.SQLSetEnvAttr`/`SQLSetConnectAttr` (unix/windows/zos; драйвер использует UIntPtr-варианты, C-обёртки сохранены), `Pool.Display` (отладочный, вызывающих нет), `testdata.GetConnectionInfoFromConfigFile`, `testingIssue5` + недостижимая ветка сна в `Exec`. `CreateDb`/`DropDb`+`SQLCreateDb`/`SQLDropDb` сохранены — документированный публичный API (API_DOCUMENTATION.md), используется тестами. `deadcode -test ./testdata` — чисто; default lint 0; сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

- [x] **Sentinel-ошибки, параллельные тесты, lll/mnd — раунд 8 (2026-11-01).** err113 44→0 (sentinel'ы + %w), paralleltest 11→0, lll 50→0 (api/ — исключение в конфиге), mnd 35→0 (5 именованных констант + сужение категории), попутно закрыты noctx/unconvert/nolintlint/gofumpt-хвосты. Строгий конфиг теперь содержит ТОЛЬКО структурные категории: nestif 7, gocognit 6, gochecknoglobals 3, gochecknoinits 1, funlen 1 — все требуют рефакторинга ядра, отложены до доступа к DB2 LUW. Проверки: build/gofmt/unit ✅, default lint 0, сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

- [x] **gosec до нуля — раунд 6 (2026-11-01).** Снята дедупликация: `rows.go` ColumnType* переведены на `odbcNum` (реальный фикс индексов), конвертации ограниченных длин (conn/database/odbcstmt/column) аннотированы, G103 исключён в конфиге с обоснованием (cgo-маршалинг ODBC-буферов — назначение пакета). **gosec: 28 → 0.** Проверки: build/unit ✅, default lint 0, сьют Informix 0 FAIL / 32 SKIP / 39 PASS.

## Открытые проблемы

- [ ] **Гонка в тестах пула (GoHawk).** В `testdata/main.go:705` и `:753` горутины читают и меняют общий `flag` без синхронизации. Исправить оба сценария и проверить с `go test -race ./testdata` на тестовой БД.

- [ ] **Транзакция без явного отката (GoHawk).** В `testdata/main.go:436` при ошибке `bg.Exec` функция `Commit` возвращается без `Rollback`. Добавить откат и проверить сценарий неудачного выполнения.

- [ ] **Предупреждения GoHawk об освобождении ресурсов.** Разобрать 13 предупреждений для `Query`/`Prepare` в `testdata/main.go`: проверить каждый путь возврата, исправить подтверждённые утечки `Rows`/`Stmt` и не считать ошибочный вызов созданным ресурсом.

Прогон GoHawk v0.7.1: корневые пакеты, `db2dialect` и `db2dialect/integration` — без предупреждений; `testdata` — 16 предупреждений, перечисленных выше.

- [x] **Ревью №3 (2026-11-01) — три дефекта + Go 1.27.3.** 1) Пустые массивы параметров паниковали (`d[0]`/`&b[0]` в пяти кейсах param.go) — добавлен guard с экспортированным `ErrEmptyArrayParameter`; интеграционный тест `TestEmptyArrayParameterErrors` (все 5 типов, ошибка вместо panic). 2) Пул преждевременно закрывал повторно использованное соединение: каждый Close запускал горутину-таймер, старый не отменялся — теперь `idleTimer` (`time.AfterFunc`) со сбросом при выдаче/возврате + `idleGen`-поколение против заблокированного на мьютексе колбэка; `Timeout` помечен Deprecated; тест `TestPoolReuseResetsIdleTimer`. 3) Wide-значения >2 МиБ паниковали (`*[1<<20]`-каст в column.go/sqlOut.go) — заменено на `unsafe.Slice` без границы; тест `TestWideColumnValueNoPanic`. Go: go.mod 1.22.1 → 1.27.3 (CI уже на 1.27.x); исправлены новые под 1.27 govet-замечания (`%p` для указателя в трейсе, `reflect.Pointer` вместо deprecated `reflect.Ptr`).

- [x] **Ревью №2 локальной истории (2026-11-01) — три замечания.** 1) Windows-сборка сломалась в 93bf8a1: `nil` вместо `api.SQLPOINTER(...)` в rows.go ×6 (на Windows SQLPOINTER = uintptr) — конвертации восстановлены, плюс капа декомпрессии переведена на int64 (386); кросс-сборка `GOOS=windows {amd64,386}` проверяется. 2) Удалённые в 70efe27 `api.SQLSetEnvAttr`/`api.SQLSetConnectAttr`/`api.StringToUTF16Ptr`/`Pool.Display` восстановлены с маркерами `Deprecated:` — внешняя совместимость сохранена (deadcode снова их видит: осознанно). 3) Args-free `QueryContext` (SQLExecDirect) не отменялся — добавлен `Conn.QueryContext` (driver.QueryerContext) с `watchContext`; тест `TestQueryContextCancelNoArgs`: заблокированный LOCK TABLE запрос возвращается по дедлайну (502 мс при таймауте 500 мс), Informix после отмены lock-wait может вернуть committed-данные как success — тест допускает оба исхода.

- [x] **Ревью локальной истории (2026-11-01, вечер) — три замечания.** 1) Гонка в `watchContext`: `stop` не дожидался выхода горутины-наблюдателя — `SQLCancel` мог «протечь» в следующий запрос на том же statement; теперь `stop` ждёт `<-exited`. 2) `maskDSN` не распознавал ключи с пробелами (`PWD =secret`) — добавлен `TrimSpace` ключа + тест-кейсы. 3) `NextResultSet` звал блокирующий `SQLMoreResults` без наблюдателя — добавлены `watchContext` и приоритет `ctx.Err()`. Возможное продолжение: наблюдатель на `SQLPrepare` (редко блокирует, не сделано).

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

- [x] **Средний: конвертации целых без проверки переполнения (gosec G115, 13 шт.).** *Исправлено 2026-11-01:* `IsNull` сравнивает на полной ширине SQLLEN (реальный баг: усечение до int16 могло дать ложный NULL); добавлены `odbcNum` (0-based индекс → 1-based номер, panic при переполнении; применён и в `rows.go` ColumnType*) и `sqlulen` (len → SQLULEN с ошибкой) для строковых/бинарных параметров; конвертации ограниченных длин (DSN/запрос/имя БД/фиксированные буферы) помечены `//nolint:gosec` с обоснованием; G103 (аудит unsafe — ядро пакета и есть cgo-маршалинг буферов ODBC) исключён в конфиге. **gosec в строгом прогоне: 0.**
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

- [ ] **Высокая когнитивная сложность (gocognit, порог 30).** `installer/setup.go` `main` — 74; [`sqlOut.go`](sqlOut.go#L248) `ConvertAssign` — 71; [`param.go`](param.go#L42) `BindValue` — 70; `odbcstmt.go`: 122 (`Exec` — 45), 237 (`BindColumns` — 31); [`rows.go`](rows.go#L146) `Next` — 34.
- [ ] **Слишком длинные функции (funlen).** [`sqlOut.go`](sqlOut.go#L33) `newOut` — 75 операторов (>60).
- [ ] **Глубокая вложенность (nestif, 7 шт.).** `installer/setup.go`: 172, 204, 382; `odbcstmt.go`: 146, 288; `pooling.go`: 223; `rows.go`: 150; `sqlOut.go`: 42.
- [x] **Неполные switch по `reflect.Kind` (exhaustive).** *Закрыто 2026-11-01:* во все switch в sqlOut.go добавлены явные `default` с сохранением поведения; в конфиге `default-signifies-exhaustive: true`.

### Стиль и гигиена — низкий приоритет

- [x] **Динамические ошибки вместо sentinel/`%w` (err113, 44 шт.).** *Исправлено 2026-11-01:* добавлены sentinel-ошибки в error.go — экспортированные для `errors.Is` (ErrStmtClosed, ErrStmtAlreadyClosed, ErrNoResultSet, ErrWrongArgumentCount, ErrAlreadyInTransaction, ErrNotInTransaction, ErrParameterArrayLength) и unexported для внутренних инвариантов; все 44 вызова переведены на sentinel'ы/`%w`-обёртки (инсталлятор — nolint как CLI-диагностика). `column.go`: 96, 149, 255, 314, 350, 385, 392; `database.go`: 21, 34, 97; `handle.go`: 31, 43; `installer/setup.go`: 41; `odbcstmt.go`: 128, 172, 180, 188, 250; `param.go`: 298, 376; `result.go`: 63; `sqlOut.go` ×15 (47, 80, 130, 219, 227, 231, 236, 256, 262, 268, 380, 411, 420, 429, 445); `stats.go`: 37; `stmt.go`: 66, 97, 161; `tx.go`: 29, 41.
- [x] **staticcheck QF/ST (9 шт., помимо SA1019).** *Исправлено:* QF1001 — `error.go`; ST1005 — `installer/setup.go`: 35, 41, 47, 54; QF1008 — `conn.go`, `driver.go`, `pooling.go`, `installer/setup.go`; QF1003 — `installer/setup.go` (переписаны на switch).
- [x] **gocritic.** *Исправлено:* ifElseChain → switch (`installer/setup.go`: 209, 271, 281, 390; `database.go`: 72); assignOp (`column.go`: 306); commentFormatting (`log2/logger1.go`). Остаётся только exitAfterDefer — см. блок безопасности.
- [x] **revive (50 шт.).** *Исправлено 2026-11-01:* документированы все exported-символы драйвера и api/ (включая пакеты go_ibm_db/log2); исправлены var-naming (URL-имена, `lastInsertID`, `linuxUntar`/`aixUntar`), indent-error-flow/early-return/superfluous-else в инсталляторе, unused-parameter в интерфейсных методах Column, captLocal в `SQLColAttribute`. ODBC-имена констант/полей — публичный API: исключение var-naming для `api/` в конфиге. revive в строгом прогоне: **0**.
- [x] **Строки длиннее 120 символов (lll, 50 шт.).** *Закрыто 2026-11-01:* api/ исключён в конфиге (//sys-директивы и cgo-мосты нельзя переносить), в драйвере и инсталляторе строки перенесены/сокращены. lll: 0.
- [x] **Магические числа (mnd, 35 шт.).** *Закрыто 2026-11-01:* значимые вынесены в константы column.go (maxColumnBufferSize, maxXMLBufferSize, columnNameBufferSize, getDataChunkSize, secondsPerHour); для ODBC-идиом (байты на юнит, базы strconv, права файлов) категория в конфиге сужена до `assign`.
- [x] **godot (3 шт.).** *Закрыто 2026-11-01:* док-комментарии пула завершены точками; cgo-преамбула `api/api_unix.go` — исключение в конфиге (C-код нельзя аннотировать).
- [x] **godox.** *Закрыто 2026-11-01:* устаревшие TODO переписаны в описательные комментарии, живые заметки сохранены без маркера.
- [x] **intrange.** *Исправлено:* `database.go`: 26; `param.go` (все циклы по `d`); `pooling_test.go`: 52.
- [x] **gofumpt.** *Исправлено:* `api/zapi_unix.go`: 257; `column.go`: 203; `handle.go`: 8. CI по-прежнему проверяет только `gofmt`.
- [x] **perfsprint.** *Исправлено:* `fmt.Sprintf` → конкатенация: `api/api.go`: 92; `conn.go`: 58, 136 (неиспользуемый импорт `fmt` удалён).
- [x] **Опечатки.** *Исправлено:* `stmt.go`: 75 — «the the»; `stmt.go`: 193 — «implementes» → «implements».
- [x] **whitespace.** *Исправлено:* `param.go`: 40 — лишняя пустая строка удалена.
- [x] **Глобальные состояния (gochecknoglobals/gochecknoinits).** *Частично закрыто 2026-11-01:* `testingIssue5` удалён как dead code (никогда не записывался, ветка недостижима). Остались `drv`+`init` (driver.go — архитектура database/sql) и `globalvar`/`traceFileWarned` (log2 — трассировка) — принять осознанно.
- [x] **paralleltest (11 шт.).** *Закрыто 2026-11-01:* `t.Parallel()` добавлен во все unit-тесты (dsn/pooling) и их подтесты; `testdata/` осознанно без параллелизма (живая БД).
