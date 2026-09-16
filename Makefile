include .env

.PHONY: db-up db-down migrate-create migrate-up migrate-down

run:
	go run cmd/server/main.go

db-up:
	docker compose up -d

db-down:
	docker compose down

migrate-create:
	migrate create -ext sql -dir migrations -seq init_schema

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down
