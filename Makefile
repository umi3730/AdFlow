.PHONY: test test-race vet verify run fmt tidy

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

verify: vet test

run:
	go run ./cmd/api

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy
