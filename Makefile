
GOLANGCI_LINT_VERSION=v1.23.7

LINTER=./bin/golangci-lint

.PHONY: build clean test lint

build:
	go build ./...

clean:
	go clean

test:
	go test -race -v ./...
	@# The proxy tests must be run separately because Go caches the global proxy environment variables. We use
	@# build tags to isolate these tests from the main test run so that if you do "go test ./..." you won't
	@# get unexpected errors.
	for tag in proxytest1 proxytest2; do go test -race -v -tags=$$tag ./proxytest; done

$(LINTER):
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s $(GOLANGCI_LINT_VERSION)

lint: $(LINTER)
	$(LINTER) run ./...
