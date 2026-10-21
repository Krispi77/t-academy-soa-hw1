.PHONY: generate build test run up down psql

# Генерация кода из openapi/openapi.yaml -> internal/api/api.gen.go
generate:
	go generate ./...

build: generate
	go build -o bin/server ./cmd/server

test: generate
	go test ./...

# Локальный запуск (PostgreSQL и миграции — из docker compose).
run: generate
	DATABASE_URL=postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable go run ./cmd/server

up:
	docker compose up --build

down:
	docker compose down -v

psql:
	docker compose exec postgres psql -U marketplace -d marketplace
