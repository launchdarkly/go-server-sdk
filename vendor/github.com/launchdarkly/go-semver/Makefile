
GOLANGCI_LINT_VERSION=v1.60.1

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

ALL_SOURCES := $(shell find * -type f -name "*.go")

COVERAGE_PROFILE_RAW=./build/coverage_raw.out
COVERAGE_PROFILE_RAW_HTML=./build/coverage_raw.html
COVERAGE_PROFILE_FILTERED=./build/coverage.out
COVERAGE_PROFILE_FILTERED_HTML=./build/coverage.html

.PHONY: build clean test test-coverage lint bump-min-go-version

bump-min-go-version:
	go mod edit -go=$(MIN_GO_VERSION) go.mod
	cd ./.github/variables && sed -i.bak "s#min=[^ ]*#min=$(MIN_GO_VERSION)#g" go-versions.env && rm go-versions.env.bak

build:
	go build ./...

clean:
	go clean

test: build
	go test ./...

benchmarks: build
	mkdir -p ./build
	go test -benchmem '-run=^$$' -bench . | tee build/benchmarks.out
	@if grep <build/benchmarks.out '[1-9][0-9]* allocs/op'; then echo "Heap allocations detected in benchmarks!"; exit 1; fi

test-coverage: $(COVERAGE_PROFILE_RAW)
	if [ -z "$(which go-coverage-enforcer)" ]; then go get -u github.com/launchdarkly-labs/go-coverage-enforcer; fi
	go-coverage-enforcer -skipcode "// COVERAGE" -showcode -outprofile $(COVERAGE_PROFILE_FILTERED) $(COVERAGE_PROFILE_RAW)
	go tool cover -html $(COVERAGE_PROFILE_FILTERED) -o $(COVERAGE_PROFILE_FILTERED_HTML)
	go tool cover -html $(COVERAGE_PROFILE_RAW) -o $(COVERAGE_PROFILE_RAW_HTML)

$(COVERAGE_PROFILE_RAW): $(ALL_SOURCES)
	mkdir -p ./build
	go test -coverprofile $(COVERAGE_PROFILE_RAW) ./...

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
