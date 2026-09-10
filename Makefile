.PHONY: test test-race integration-test vet verify frontend-verify browser-test fmt-check run migrate fmt tidy

# Exclude ignored work/ experiments from reproducible project checks.
GO_PACKAGES := ./cmd/... ./internal/... ./tests/...

test:
	go test $(GO_PACKAGES)

test-race:
	go test -race $(GO_PACKAGES)

integration-test:
	go test -tags=integration -v ./tests/integration

vet:
	go vet $(GO_PACKAGES)

verify: fmt-check vet test frontend-verify

frontend-verify:
	cd web && npm run lint && npm test

browser-test:
	cd web && npm run test:e2e

fmt-check:
	@files="$$(gofmt -l cmd internal tests)" || exit $$?; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi

run:
	go run ./cmd/api

migrate:
	go run ./cmd/migrate -dir migrations

fmt:
	gofmt -w cmd internal tests

tidy:
	go mod tidy
