
GOLANGCI_LINT_VERSION=v1.57.1

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

GO_WORK_FILE=go.work
GO_WORK_SUM=go.work.sum

TEST_BINARY=./go-server-sdk.test
ALLOCATIONS_LOG=./build/allocations.out

OUTPUT_DIR=./build

ALL_SOURCES := $(shell find * -type f -name "*.go")

COVERAGE_PROFILE_RAW=./build/coverage_raw.out
COVERAGE_PROFILE_RAW_HTML=./build/coverage_raw.html
COVERAGE_PROFILE_FILTERED=./build/coverage.out
COVERAGE_PROFILE_FILTERED_HTML=./build/coverage.html
COVERAGE_ENFORCER_FLAGS=-package github.com/launchdarkly/go-server-sdk/v7 \
	-skipfiles '(internal/sharedtest/$(COVERAGE_ENFORCER_SKIP_FILES_EXTRA))' \
	-skipcode "// COVERAGE" \
	-packagestats -filestats -showcode

ALL_BUILD_TARGETS=sdk ldotel
ALL_TEST_TARGETS = $(addsuffix -test, $(ALL_BUILD_TARGETS))
ALL_LINT_TARGETS = $(addsuffix -lint, $(ALL_BUILD_TARGETS))

.PHONY: all build clean test test-coverage benchmarks benchmark-allocs lint workspace workspace-clean $(ALL_BUILD_TARGETS) $(ALL_TEST_TARGETS) $(ALL_LINT_TARGETS)

all: $(ALL_BUILD_TARGETS)

test: $(ALL_TEST_TARGETS)

clean: workspace-clean
	rm -rf ./bin/

sdk:
	go build ./...

sdk-test:
	go test -v -race ./...
	@# The proxy tests must be run separately because Go caches the global proxy environment variables. We use
	@# build tags to isolate these tests from the main test run so that if you do "go test ./..." you won't
	@# get unexpected errors.
	for tag in proxytest1 proxytest2; do go test -race -v -tags=$$tag ./proxytest; done

sdk-lint:
	$(LINTER) run ./...

ldotel:
	go build ./ldotel

ldotel-test:
	go test -v -race ./ldotel

ldotel-lint:
	$(LINTER) run ./ldotel

test-coverage: $(COVERAGE_PROFILE_RAW)
	go run github.com/launchdarkly-labs/go-coverage-enforcer@latest $(COVERAGE_ENFORCER_FLAGS) -outprofile $(COVERAGE_PROFILE_FILTERED) $(COVERAGE_PROFILE_RAW)
	go tool cover -html $(COVERAGE_PROFILE_FILTERED) -o $(COVERAGE_PROFILE_FILTERED_HTML)
	go tool cover -html $(COVERAGE_PROFILE_RAW) -o $(COVERAGE_PROFILE_RAW_HTML)

$(COVERAGE_PROFILE_RAW): $(ALL_SOURCES)
	@mkdir -p ./build
	go test -coverprofile $(COVERAGE_PROFILE_RAW) -coverpkg=./... ./... >/dev/null
	# note that -coverpkg=./... is necessary so it aggregates coverage to include inter-package references

benchmarks:
	@mkdir -p ./build
	go test -benchmem '-run=^$$' -bench . ./... | tee build/benchmarks.out
	@if grep <build/benchmarks.out 'NoAlloc.*[1-9][0-9]* allocs/op'; then echo "Unexpected heap allocations detected in benchmarks!"; exit 1; fi

# See CONTRIBUTING.md regarding the use of the benchmark-allocs target. Notes about this implementation:
# 1. We precompile the test code because otherwise the allocation traces will include the actions of the compiler itself.
# 2. "benchtime=3x" means the run count (b.N) is set to 3. Setting it to 1 would produce less redundant output, but the
#    benchmark statistics seem to be less reliable if the run count is less than 3 - they will report a higher allocation
#    count per run, possibly because the first run
benchmark-allocs:
	@if [ -z "$$BENCHMARK" ]; then echo "must specify BENCHMARK=" && exit 1; fi
	@mkdir -p $(OUTPUT_DIR)
	@echo Precompiling test code to $(TEST_BINARY)
	@go test -c -o $(TEST_BINARY) >/dev/null 2>&1
	@echo "Generating heap allocation traces in $(ALLOCATIONS_LOG) for benchmark(s): $$BENCHMARK"
	@echo "You should see some benchmark result output; if you do not, you may have misspelled the benchmark name/regex"
	@GODEBUG=allocfreetrace=1 LD_TEST_ALLOCATIONS= $(TEST_BINARY) -test.run=none -test.bench=$$BENCHMARK -test.benchmem -test.benchtime=1x 2>$(ALLOCATIONS_LOG)

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE) $(ALL_LINT_TARGETS)

TEMP_TEST_OUTPUT=/tmp/sdk-contract-test-service.log

# TEST_HARNESS_PARAMS can be set to add -skip parameters for any contract tests that cannot yet pass
TEST_HARNESS_PARAMS=

workspace: go.work

go.work:
	go work init ./
	go work use ./ldotel
	go work use ./testservice

workspace-clean:
	rm -f $(GO_WORK_FILE) $(GO_WORK_SUM)

build-contract-tests:
	@go build -o ./testservice/testservice ./testservice

start-contract-test-service: build-contract-tests
	@./testservice/testservice

start-contract-test-service-bg:
	@echo "Test service output will be captured in $(TEMP_TEST_OUTPUT)"
	@make start-contract-test-service >$(TEMP_TEST_OUTPUT) 2>&1 &

run-contract-tests:
	@curl -s https://raw.githubusercontent.com/launchdarkly/sdk-test-harness/v2/downloader/run.sh \
      | VERSION=v2 PARAMS="-url http://localhost:8000 -debug -stop-service-at-end $(TEST_HARNESS_PARAMS)" sh

contract-tests: build-contract-tests start-contract-test-service-bg run-contract-tests

.PHONY: build-contract-tests start-contract-test-service start-contract-test-service-bg run-contract-tests contract-tests
