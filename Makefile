# Makefile
export GO111MODULE := on
export REPO_NAME := liqbot

ifeq ($(CI),)
	# Not in CI
	VERSION := dev-$(USER)
	VERSION_HASH := $(shell git rev-parse HEAD | cut -b1-8)
else
	# In CI
	ifneq ($(RELEASE_VERSION),)
		VERSION := $(RELEASE_VERSION)
	else
		# No tag, so make one
		VERSION := $(shell git describe --tags 2>/dev/null)
	endif
	VERSION_HASH := $(shell echo "$(GITHUB_SHA)" | cut -b1-8)
endif

GO_FLAGS := -ldflags "-X main.Version=$(VERSION) -X main.VersionHash=$(VERSION_HASH)"

.PHONY: all
default: deps build test lint

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

.PHONY: install
install:
	@go install $(GO_FLAGS) ./cmd/${REPO_NAME}

.PHONY: release-ubuntu-latest
release-ubuntu-latest:
	@mkdir -p build
	@env GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -v -o build/${REPO_NAME}-linux-amd64 $(GO_FLAGS) ./cmd/${REPO_NAME}
	@cd build && zip ${REPO_NAME}-linux-amd64.zip ${REPO_NAME}-linux-amd64

.PHONY: release-macos-latest
release-macos-latest:
	@mkdir -p build
	@env GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -v -o build/${REPO_NAME}-darwin-amd64 $(GO_FLAGS) ./cmd/${REPO_NAME}
	@cd build && zip ${REPO_NAME}-darwin-amd64.zip ${REPO_NAME}-darwin-amd64

.PHONY: release-windows-latest
release-windows-latest:
	@env GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -v -o build/${REPO_NAME}-amd64.exe $(GO_FLAGS) ./cmd/${REPO_NAME}
	@cd build && 7z a -tzip ${REPO_NAME}-windows-amd64.zip ${REPO_NAME}-amd64.exe


.PHONY: build
build: ## install the binary in GOPATH/bin
	@go build -v -o bin/${REPO_NAME} ./cmd/${REPO_NAME}

.PHONY: lint
lint:
	@golangci-lint run -v --config .golangci.toml

.PHONY: mocks
mocks: ## Make mocks
	@find -name '*_mock.go' -print0 | xargs -0r rm
	@go generate ./...

.PHONY: race
race: ## Run data race detector
	@env CGO_ENABLED=1 go test -race ./...

.PHONY: retest
retest: ## Force re-run of all tests
	@go test -count=1 ./...

.PHONY: test
test: ## Run tests
	@go test ./...

.PHONY: clean
clean: ## Remove previous build
	@rm -f ./bin/${REPO_NAME}

.PHONY: help
help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
