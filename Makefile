.PHONY: dev test lint build migrate generate clean help

dev:
	@bash scripts/dev.sh

test:
	go test ./... -v -cover -race

lint:
	golangci-lint run

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/server ./cmd/server

migrate:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

generate:
	go generate ./internal/infrastructure/persistence/ent

swagger:
	swag init -g cmd/server/main.go -o api/swagger

clean:
	rm -rf bin/
	rm -rf web/dist/

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

docker-build:
	docker compose -f deploy/docker/docker-compose.yml build

docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

help:
	@echo "Available targets:"
	@echo "  dev           - Start development environment"
	@echo "  test          - Run tests"
	@echo "  lint          - Run linter"
	@echo "  build         - Build binary"
	@echo "  migrate       - Run database migrations"
	@echo "  migrate-down  - Rollback last migration"
	@echo "  generate      - Generate Ent code"
	@echo "  swagger       - Generate Swagger docs"
	@echo "  clean         - Clean build artifacts"
	@echo "  web-dev       - Start frontend dev server"
	@echo "  web-build     - Build frontend"
	@echo "  docker-build  - Build Docker images"
	@echo "  docker-up     - Start Docker services"
	@echo "  docker-down   - Stop Docker services"
