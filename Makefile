
GOLANGCI_LINT_VERSION=v1.27.0

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

TEST_BINARY=./go-server-sdk.test

OUTPUT_DIR=./build
ALLOCATIONS_LOG=$(OUTPUT_DIR)/allocations.out
COVERAGE_LOG=$(OUTPUT_DIR)/coverage.out
COVERAGE_HTML=$(OUTPUT_DIR)/coverage.html

.PHONY: build clean test test-coverage benchmarks benchmark-allocs lint

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

test-coverage:
	@mkdir -p $(OUTPUT_DIR)
	@echo "Note that the percentage figures in the output from \"go test\" below are not the percentage of"
	@echo "covered code in each package, but rather the percentage as compared to all code in the project."
	@echo "Those numbers are not useful; instead, look at the HTML report and the script results that"
	@echo "appear after this section."
	@echo
	go test -coverpkg ./... -coverprofile $(COVERAGE_LOG) ./...
	go tool cover -html $(COVERAGE_LOG) -o $(COVERAGE_HTML)

benchmarks:
	go test -benchmem '-run=^$$' gopkg.in/launchdarkly/go-server-sdk.v5 -bench .

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
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
