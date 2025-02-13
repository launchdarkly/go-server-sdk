GOLANGCI_LINT_VERSION=v1.60.1

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

SHELL=/bin/bash

build:
	go build ./...

test:
	go test -race -v ./...

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...

TEMP_TEST_OUTPUT=/tmp/sse-contract-test-service.log

bump-min-go-version:
	go mod edit -go=$(MIN_GO_VERSION) go.mod
	cd contract-tests && go mod edit -go=$(MIN_GO_VERSION) go.mod
	cd ./.github/variables && sed -i.bak "s#min=[^ ]*#min=$(MIN_GO_VERSION)#g" go-versions.env && rm go-versions.env.bak

build-contract-tests:
	@cd contract-tests && go mod tidy && go build

start-contract-test-service:
	@./contract-tests/contract-tests

start-contract-test-service-bg:
	@echo "Test service output will be captured in $(TEMP_TEST_OUTPUT)"
	@make start-contract-test-service >$(TEMP_TEST_OUTPUT) 2>&1 &

run-contract-tests:
	@curl -s https://raw.githubusercontent.com/launchdarkly/sse-contract-tests/v2.0.0/downloader/run.sh \
      | VERSION=v2 PARAMS="-url http://localhost:8000 -debug -stop-service-at-end" sh

contract-tests: build-contract-tests start-contract-test-service-bg run-contract-tests

.PHONY: build lint test build-contract-tests start-contract-test-service run-contract-tests contract-tests bump-min-go-version
