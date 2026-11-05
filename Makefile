ifneq (,$(wildcard .env.example))
include .env.example
export
endif
ifneq (,$(wildcard .env))
include .env
export
endif

GOLANGCI_LINT_VERSION ?= v2.5.0
COVERPKG := ./internal/usecase/...,./internal/domain/...

.PHONY: generate migrate migrate-down run test test-integration lint

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

test:
	go test -race -count=1 -coverpkg=$(COVERPKG) -coverprofile=cover.out ./...
	@go tool cover -func=cover.out | awk '/^total:/{print "coverage (domain+usecase): " $$3}'

test-integration:
	go test -race -count=2 -tags=integration ./internal/repository/...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...
