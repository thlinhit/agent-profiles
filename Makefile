.PHONY: check hooks

# Same checks run by CI and by the pre-push check.
check:
	go vet ./...
	go test ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping lint"; \
	fi

# One-time setup: point git at the tracked hooks directory.
hooks:
	git config core.hooksPath .githooks
