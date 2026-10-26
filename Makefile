ifneq (,$(wildcard .env.example))
include .env.example
export
endif
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: generate migrate migrate-down run test lint

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

test:
	go test -race ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint is not installed (required from lab 2)"
