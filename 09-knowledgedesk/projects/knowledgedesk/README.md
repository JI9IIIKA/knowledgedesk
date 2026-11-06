# KnowledgeDesk — База знаний с поиском и RAG

Проект №09. Самостоятельное Go-приложение. Сборка: `./cmd/knowledgedesk`.
Общие пакеты находятся в `internal/platform`, поэтому сохраняйте структуру репозитория.

## Реализовано в v0.1

Загрузка текста; версии; асинхронная индексация; разбиение на фрагменты; полнотекстовый поиск PostgreSQL; ACL до обращения к AI; ссылки на источники; LangChain-генерация при подключённом AI-сервисе.

Данные: `documents, document_access, chunks`. Схема: `../../migrations/knowledgedesk_001.sql`.
Все записи изолированы по tenant; роль определяется серверным API-токеном.

## Запуск

Из корня репозитория:

```bash
python3 scripts/setup.py  # один раз для всего набора
PROJECT=knowledgedesk docker compose up --build -d app
python3 scripts/demo.py knowledgedesk
```

Для JobRunner запускайте ещё `--profile jobs`; точные команды профилей — в корневом README.
Демо создаёт тестовые записи. Остановить: `docker compose down`.

## API

Все пути требуют `Authorization: Bearer <token>`. Ошибки возвращаются как JSON.
`Idempotency-Key` необязателен, но рекомендуется для команд создания и отправки.
Повтор того же запроса с тем же ключом возвращает сохранённый ответ; изменение запроса даёт 409.

| Метод | Путь |
|---|---|
| POST | `/documents` |
| GET | `/documents` |
| PUT | `/documents/{id}` |
| POST | `/documents/{id}/access` |
| DELETE | `/documents/{id}/access/{user}` |
| DELETE | `/documents/{id}` |
| POST | `/ask` |

## Примеры тел запросов

`POST /documents`:

```json
{
  "title": "Правила",
  "content": "Возврат оформляется через поддержку.",
  "public": false
}
```

`PUT /documents/{id}`:

```json
{
  "expected_version": 1,
  "title": "Правила",
  "content": "Обновлённый текст",
  "public": false
}
```

`POST /documents/{id}/access`:

```json
{
  "user_id": "bob"
}
```

`POST /ask`:

```json
{
  "question": "возврат"
}
```

## Проверка

```bash
TEST_DATABASE_URL='postgres://portfolio:portfolio-local-only@localhost:5432/portfolio?sslmode=disable' \
  go test -count=1 -v ./tests -run '^TestKnowledgeDesk$'
```

Интеграционные тесты пишут данные в указанную БД. Используйте отдельную тестовую базу.
Полный сценарий проверки находится в `../../tests/integration_test.go`.

## Границы версии и дальнейшая работа

Парсинг PDF/DOCX, pgvector/эмбеддинги, гибридный поиск и автоматическая оценка качества. Сейчас поиск лексический; без AI_URL возвращаются только источники и честный retrieval_only-статус.

Это реализованное ядро проекта, а не заявление о завершении всех расширений исходной идеи.
Подробнее об ограничениях окружения проверки — `../../VALIDATION.md`.
