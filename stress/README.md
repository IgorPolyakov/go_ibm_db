# Локальный стресс и фаззинг драйвера

`TestLiveStress` запускает 16 сценариев параллельно, а `FuzzLiveSequence` меняет их порядок. Они проверяют открытие и закрытие соединений, прямые и подготовленные `Query`/`Exec`, параметры и массивы, `Rows` и метаданные, `RowsAffected`/`LastInsertId`, `GetInfo`, `Ping`, отменённый контекст, `Begin`/`Commit`/`Rollback`, legacy-интерфейсы драйвера и собственный пул. Обычный `FETCHSIZE` чередуется с `FETCHSIZE=8`. Для записей каждый воркер создаёт собственную временную таблицу; глобальные таблицы не затрагиваются.

Тесты пропускаются без `DB2_STRESS_DSN`, поэтому обычный `go test ./...` не обращается к БД. Административные `CreateDb`/`DropDb` и `Driver.Close` не входят в нагрузку: первые меняют базу целиком, второй закрывает глобальный драйвер процесса. `sql.Out` требует отдельной процедуры на целевой СУБД и пока не покрыт.

## Локальный Informix

Нужны установленный clidriver и доступный контейнер `sendsay-ifx-probe`. Скрипт определяет опубликованный DRDA-порт через `docker port`. Пароль не хранится в репозитории:

```bash
export IFX_PASSWORD='...'
export IBM_DB_HOME=/path/to/clidriver
scripts/stress-informix.sh both
```

Аргументы `stress` и `fuzz` запускают режимы по отдельности. `DB2_STRESS_DSN` позволяет передать готовый DSN вместо `IFX_PASSWORD`; `IFX_PORT` задаёт порт без Docker. Скрипт подставляет запросы к `sysmaster` и создаёт только сессионные `TEMP` таблицы.

Настройки: `DB2_STRESS_DURATION` (`2m`), `DB2_STRESS_WORKERS` (`8`), `DB2_STRESS_SEED` (`1`), `DB2_STRESS_FUZZTIME` (`1m`), `DB2_STRESS_FUZZ_WORKERS` (`4`) и `DB2_STRESS_TEST_TIMEOUT` (`5m`). При ошибке нагрузочного теста сохраняйте `seed`, номер воркера, итерацию и операцию. Фаззер сохраняет минимизированный вход в `stress/testdata/fuzz/FuzzLiveSequence/`; воспроизведение: `go test -race ./stress -run 'FuzzLiveSequence/<имя_файла>'` с тем же DSN и настройками.

Для DB2 задайте `DB2_STRESS_DSN`, `DB2_STRESS_QUERY` и при необходимости `DB2_STRESS_ROWS_QUERY`. Запросы должны возвращать целые числа; второй читает до 32 строк за операцию. Для Informix задайте `DB2_STRESS_PLATFORM=INFORMIX`. На DB2 по умолчанию используются временные таблицы `SESSION`; SQL для LUW, z/OS и IBM i ещё не проверен на живых серверах.

`-race` обнаруживает гонки Go и включает `checkptr`, но не видит внутренние гонки C-библиотеки. Текущий базовый прогон на Informix с `-race` останавливается на `checkptr` в `ODBCStmt.BindColumns` при `FETCHSIZE=8`; см. `todo.md`. До исправления этого дефекта длительная нагрузка не начнётся.
