VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/cordon-co/cordon-cli/cli/internal/buildinfo.Version=$(VERSION)"
BUILD   := build
OPENAPI_SPEC ?= ../openapi/cordon-v1.openapi.yaml
OPENAPI_CONFIG := cli/internal/apicontract/oapi-codegen.yaml
OPENAPI_OUTPUT := cli/internal/apicontract/gen_types.go

.PHONY: build build-all clean fmt vet \
        build-darwin-arm64 build-darwin-amd64 \
        build-linux-amd64 build-linux-arm64 \
        openapi-generate openapi-check

## build: compile for the current platform
build:
	go build $(LDFLAGS) -o $(BUILD)/cordon ./cmd/cordon

## build-all: cross-compile for all release targets
build-all: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD)/cordon-darwin-arm64 ./cmd/cordon

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD)/cordon-darwin-amd64 ./cmd/cordon

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD)/cordon-linux-amd64 ./cmd/cordon

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD)/cordon-linux-arm64 ./cmd/cordon

## fmt: format all Go source
fmt:
	go fmt ./cmd/cordon/...

## vet: run go vet
vet:
	go vet ./cmd/cordon/...

## clean: remove build artifacts
clean:
	rm -rf $(BUILD)

## openapi-generate: generate API contract types from OpenAPI spec
openapi-generate:
	@test -f "$(OPENAPI_SPEC)" || (echo "missing OpenAPI spec at $(OPENAPI_SPEC)"; exit 1)
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 \
		-config $(OPENAPI_CONFIG) \
		$(OPENAPI_SPEC)

## openapi-check: verify generated contract types are up to date
openapi-check: openapi-generate
	@git diff --exit-code -- $(OPENAPI_OUTPUT) >/dev/null || \
		(echo "OpenAPI generated file is out of date: $(OPENAPI_OUTPUT)"; git --no-pager diff -- $(OPENAPI_OUTPUT); exit 1)
