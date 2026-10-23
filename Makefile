# Build, test and package the three entrypoints: platform-core and
# market-ingestion (Go) and research-worker (Python). Toolchains: exactly the Go
# release on go.mod's go line (go.mod alone is only a minimum, so GOTOOLCHAIN
# pins it; the go command downloads and verifies it if missing); uv and CPython
# from scripts/toolchain.sh, inside .tools/ only. See README "Build and test".
.PHONY: toolchain build test test-package arch check package reproducible integration local run dev-stack dev-stack-smoke \
	ci ci-go ci-python ci-docs ci-security ci-package ci-report

export GOTOOLCHAIN := go$(shell awk '$$1 == "go" {print $$2}' go.mod)

toolchain:
	./scripts/toolchain.sh

build:
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/platform-core ./cmd/market-ingestion

# Unit tests; PostgreSQL tests skip without TEST_DATABASE_URL (see integration).
test: toolchain
	TS_PYTHON="$$(./scripts/toolchain.sh python)" TS_REQUIRE_PYTHON=1 go test -race -count=1 ./...
	.tools/research-venv/bin/python -I -B -m unittest discover -s research/tests -t research/tests
	.tools/research-venv/bin/python -I -B -m unittest discover -s contracts/tools -t contracts/tools
	.tools/research-venv/bin/python -I -B contracts/tools/generate_contracts.py --check
	.tools/research-venv/bin/python -I -B fixtures/synthetic/test_fixtures.py
	python3 -B scripts/test_disposable_api.py

# Script safety tests: package output guard, toolchain paths, development stack
# commands and its API client. They use only folders, servers and record-only
# tools each test creates; no container, volume or real API is touched.
# Overrides used only by deliberate red controls are cleared here.
test-package:
	env -u PACKAGE_OUT_LIBRARY python3 -B scripts/test_package_out.py
	python3 -B scripts/test_toolchain_paths.py
	python3 -B scripts/test_dev_stack.py
	env -u DEV_STACK_API_CLIENT python3 -B scripts/test_dev_stack_api.py

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

# CI baseline (S01-T03). .github/workflows/ci.yml calls these same targets, one
# per job; `make ci` runs them all locally. Results (logs, counts) go to a new
# folder, CI_OUT; nothing is deleted. Overrides used only by deliberate red
# controls are cleared for every suite.
CI_OUT ?= .local/ci/$(shell date -u +%Y%m%dT%H%M%SZ)
PY := .tools/research-venv/bin/python
SUITE := env -u PACKAGE_OUT_LIBRARY -u DEV_STACK_API_CLIENT python3 -B scripts/ci_suite.py run --out $(CI_OUT)
GITLEAKS := github.com/zricethezav/gitleaks/v8@v8.30.1
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.9.0

ci: ci-go ci-python ci-docs ci-security ci-package ci-report

# Go format, vet and race tests. PostgreSQL tests run only with TEST_DATABASE_URL
# (CI sets it); without it the report lists them as skipped, with the reason.
ci-go: toolchain
	@files=$$(gofmt -l $$(git ls-files '*.go' | grep -v '^spikes/')); \
	  if [ -n "$$files" ]; then echo "gofmt needed: $$files"; exit 1; fi
	go vet ./...
	# Generated Go/Python contract code is current and no contract change breaks the lock.
	$(PY) -I -B contracts/tools/generate_contracts.py --check
	TS_PYTHON="$$(./scripts/toolchain.sh python)" TS_REQUIRE_PYTHON=1 $(SUITE) --name go --go-json -- go test -race -count=1 -json ./...

# Python lint (ruff.toml) and type check (mypy.ini) with the locked tools, then
# every Python test suite.
ci-python: toolchain
	.tools/research-venv/bin/ruff check --no-cache --config ruff.toml .
	.tools/research-venv/bin/mypy --config-file mypy.ini --no-incremental --cache-dir=/dev/null
	$(SUITE) --name research -- $(PY) -I -B -m unittest discover -s research/tests -t research/tests
	$(SUITE) --name contracts -- $(PY) -I -B -m unittest discover -s contracts/tools -t contracts/tools
	$(SUITE) --name fixtures -- $(PY) -I -B fixtures/synthetic/test_fixtures.py
	$(SUITE) --name launcher-helpers -- python3 -B scripts/test_disposable_api.py
	$(SUITE) --name package-guard -- python3 -B scripts/test_package_out.py
	$(SUITE) --name toolchain-paths -- python3 -B scripts/test_toolchain_paths.py
	$(SUITE) --name dev-stack -- python3 -B scripts/test_dev_stack.py
	$(SUITE) --name dev-stack-api -- python3 -B -W error::ResourceWarning scripts/test_dev_stack_api.py
	$(SUITE) --name scan-checks -- $(PY) -I -B scripts/test_scan_checks.py
	$(SUITE) --name ci-suite -- python3 -B scripts/test_ci_suite.py

# Plan integrity, task/state schema and generated documents.
ci-docs:
	python3 -B docs/tools/planctl.py validate
	python3 -B docs/tools/planctl.py render --check
	$(SUITE) --name planctl -- python3 -B docs/tools/test_planctl.py
	$(SUITE) --name install-docs -- python3 -B test_install_docs.py

# Secret scan of the committed history (redacted; only Git objects are read, so
# ignored local files are never scanned), Go and Python dependency scans, and
# the reviewed waivers in security/scan-waivers.json.
ci-security: toolchain
	mkdir -p $(CI_OUT)
	$(PY) -I scripts/scan_checks.py waivers
	go run $(GITLEAKS) git . --redact --no-banner --exit-code 0 --report-format json --report-path $(CI_OUT)/gitleaks.json
	$(PY) -I scripts/scan_checks.py gitleaks $(CI_OUT)/gitleaks.json
	go run $(GOVULNCHECK) -format json ./... > $(CI_OUT)/govulncheck.json
	$(PY) -I scripts/scan_checks.py govulncheck $(CI_OUT)/govulncheck.json
	$(PY) -I scripts/scan_checks.py osv research/uv.lock tools/uv-requirements.txt

# Both binaries, the wheel and its smoke checks into a new folder, then the
# two-build reproducibility check.
ci-package: toolchain
	mkdir -p $(CI_OUT)
	CGO_ENABLED=0 go build -trimpath -o $(CI_OUT)/bin/ ./cmd/platform-core ./cmd/market-ingestion
	./scripts/package.sh $(CI_OUT)/package
	TMPDIR="$$(cd $(CI_OUT) && pwd)" ./scripts/check-reproducible.sh

ci-report:
	python3 -B scripts/ci_suite.py report $(CI_OUT)

# Local development stack in its own Compose project (see README).
dev-stack:
	./scripts/dev-stack.sh up
dev-stack-smoke:
	./scripts/dev-stack-smoke.sh

local:
	./scripts/setup-local.sh
run:
	./scripts/run-local.sh
integration:
	./scripts/test-integration.sh
