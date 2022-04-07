# Makefile
export GO111MODULE := on

.PHONY: default
default: install test

.PHONY: coverage
coverage:
	@go test -covermode=count -coverprofile="coverage.txt" ./...
	@go tool cover -func="coverage.txt"

.PHONY: coveragehtml
coveragehtml: coverage
	@go tool cover -html=coverage.txt -o coverage.html

.PHONY: deps
deps: ## Get the dependencies
	@go mod download

.PHONY: gosec
gosec:
	gosec ./...

.PHONY: build
build: ## install the binary in GOPATH/bin
	go build -v -o bin/traderbot ./cmd/traderbot

.PHONY: lint
lint:
	golangci-lint run -v --config .golangci.toml

.PHONY: mocks
mocks: ## Make mocks
	@go generate ./...

.PHONY: race
race: ## Run data race detector
	@env CGO_ENABLED=1 go test -race ./...

.PHONY: retest
retest: ## Force re-run of all tests
	@go test -count=1 ./...

.PHONY: staticcheck
staticcheck: ## Run statick analysis checks
	@staticcheck ./...

.PHONY: test
test: ## Run tests
	@go test ./...

.PHONY: vet
vet: deps
	@go vet ./...

.PHONY: clean
clean: ## Remove previous build
	@rm -f ./traderbot ./cmd/traderbot/traderbot

.PHONY: help
help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
