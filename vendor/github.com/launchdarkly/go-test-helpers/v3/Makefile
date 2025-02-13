
GOLANGCI_LINT_VERSION=v1.48.0

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

.PHONY: build clean test lint

build:
	go build ./...

clean:
	go clean

test: build
	go test -race -v ./...

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
