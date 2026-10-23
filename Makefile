# Build, test and package the three entrypoints: platform-core and
# market-ingestion (Go) and research-worker (Python). Toolchains: exactly the Go
# release on go.mod's go line (go.mod alone is only a minimum, so GOTOOLCHAIN
# pins it; the go command downloads and verifies it if missing); uv and CPython
# from scripts/toolchain.sh, inside .tools/ only. See README "Build and test".
.PHONY: toolchain build test test-package arch check package reproducible integration local run

export GOTOOLCHAIN := go$(shell awk '$$1 == "go" {print $$2}' go.mod)

toolchain:
	./scripts/toolchain.sh

build:
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/platform-core ./cmd/market-ingestion

# Unit tests; PostgreSQL tests skip without TEST_DATABASE_URL (see integration).
test: toolchain
	TS_PYTHON="$$(./scripts/toolchain.sh python)" TS_REQUIRE_PYTHON=1 go test -race -count=1 ./...
	.tools/research-venv/bin/python -I -B -m unittest discover -s research/tests -t research/tests
	python3 -B scripts/test_disposable_api.py

# Package output guard and toolchain path tests; they use only folders each test
# creates, with fake tools.
test-package:
	python3 -B scripts/test_package_out.py
	python3 -B scripts/test_toolchain_paths.py

# Dependency direction (Go) and package import boundary (Python) only.
arch: toolchain
	go test -count=1 ./tests/architecture/
	.tools/research-venv/bin/python -I -B -m unittest discover -s research/tests -t research/tests -p test_boundaries.py

check:
	test -z "$$(gofmt -l $$(git ls-files '*.go' | grep -v '^spikes/'))"
	go vet ./...
	python3 -B docs/tools/planctl.py validate
	python3 -B docs/tools/planctl.py render --check

package:
	./scripts/package.sh dist

reproducible:
	./scripts/check-reproducible.sh

local:
	./scripts/setup-local.sh
run:
	./scripts/run-local.sh
integration:
	./scripts/test-integration.sh
