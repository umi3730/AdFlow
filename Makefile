.PHONY: test test-race integration-test vet verify run migrate fmt tidy

test:
	go test ./...

test-race:
	go test -race ./...

integration-test:
	go test -tags=integration -v ./tests/integration

vet:
	go vet ./...

verify: vet test

run:
	go run ./cmd/api

migrate:
	go run ./cmd/migrate -dir migrations

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy
