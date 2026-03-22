VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/cordon-co/cordon-cli/cli/cmd.Version=$(VERSION)"
BUILD   := build

.PHONY: build build-all clean fmt vet \
        build-darwin-arm64 build-darwin-amd64 \
        build-linux-amd64 build-linux-arm64

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
