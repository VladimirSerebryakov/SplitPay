-include .env

LOCAL_COMPOSE := docker compose -f docker-compose.yml -f docker-compose.local.yml

.PHONY: run db-up db-down docker-build docker-up docker-down docker-logs migrate-create migrate-up migrate-down

run:
	go run ./cmd/api

db-up:
	$(LOCAL_COMPOSE) up -d postgres

db-down:
	$(LOCAL_COMPOSE) down

docker-build:
	docker build -t splitpay-api:local .

docker-up:
	$(LOCAL_COMPOSE) up -d --build api

docker-down:
	$(LOCAL_COMPOSE) stop api

docker-logs:
	$(LOCAL_COMPOSE) logs --tail=100 -f api

migrate-create:
	migrate create -ext sql -dir migrations -seq init_schema

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down
