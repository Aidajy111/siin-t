# SIIN

Сервис коротких ссылок.

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white) ![Postgres](https://img.shields.io/badge/postgres-%23316192.svg?style=for-the-badge&logo=postgresql&logoColor=white) ![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?style=for-the-badge&logo=docker&logoColor=white)

## Структура

```text
cmd/main.go                    точка входа, конфигурация и сигналы завершения
internal/
  config/config.go             чтение environment variables
  handler/health.go            HTTP-обработчики
  server/server.go             маршруты и жизненный цикл HTTP-сервера
  service/doc.go               место для бизнес-логики ссылок
  repository/postgres/doc.go   место для SQL-запросов
migrations/                    SQL-миграции up/down
Dockerfile                     сборка и образ приложения
docker-compose.yml             приложение, PostgreSQL и миграции
.env.example                   пример локальных настроек
```

## Запуск через Docker Compose

Нужен Docker с Compose v2

Создать локальную конфигурацию в PowerShell:

```powershell
Copy-Item .env.example .env
```

В Linux/macOS: `cp .env.example .env`.

Проверить конфигурацию без сборки и запуска контейнеров:

```shell
docker compose config --quiet
```

Запустить проект:

```shell
docker compose up --build -d
```

Compose сначала ожидает готовности PostgreSQL, затем применяет миграции
и только после их успешного завершения запускает приложение.
Контейнер `migrate` выполняется однократно; состояние `Exited (0)` нормально.

Проверить HTTP-сервер:

```shell
curl.exe http://localhost:8080/health
```

Для Linux/macOS используйте `curl` вместо `curl.exe`.
Ожидаемый ответ: `{"status":"ok"}`.

Посмотреть состояние и логи:

```shell
docker compose ps -a
docker compose logs app postgres migrate
```

Остановить проект:

```shell
docker compose down
```

Данные PostgreSQL остаются в именованном томе `postgres_data`.
Приложение получает SIGTERM и даёт текущим HTTP-запросам до 10 секунд
на завершение. Compose ждёт до 15 секунд перед принудительной остановкой.

## PostgreSQL и миграции отдельно

Запустить только базу:

```shell
docker compose up -d postgres
```

Применить миграции вручную, в том числе после добавления новых файлов:

```shell
docker compose run --rm migrate up
```

Проверить версию схемы:

```shell
docker compose run --rm migrate version
```

Откатить одну миграцию:

```shell
docker compose run --rm migrate down 1
```

Откат первой миграции удалит таблицу `links` вместе с её данными.

Миграциями управляет [golang-migrate](https://github.com/golang-migrate/migrate).
Файлы имеют вид `000001_create_links.up.sql` и
`000001_create_links.down.sql`. Уже применённые миграции не редактируем:
изменения схемы добавляем новой парой файлов.

В первой миграции: `id` — первичный ключ, `url` — исходный адрес,
`clicks` — неотрицательный BIGINT со значением 0 по умолчанию,
`created_at` — TIMESTAMPTZ.