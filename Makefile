.PHONY: test check local run integration
check:
	go vet ./...
	python3 -B docs/tools/planctl.py validate
	python3 -B docs/tools/planctl.py render --check
test:
	go test -race ./...
	python3 -B -m unittest discover -s research -p 'test_*.py'
	python3 -B scripts/test_disposable_api.py
local:
	./scripts/setup-local.sh
run:
	./scripts/run-local.sh
integration:
	./scripts/test-integration.sh
