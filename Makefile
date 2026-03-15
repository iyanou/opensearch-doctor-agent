VERSION ?= 0.1.0
BINARY  = opensearch-doctor-agent
BUILD_DIR = bin

.PHONY: build build-all run test clean

build:
	go build -ldflags="-s -w -X main.AgentVersion=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/agent

build-all:
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-amd64     ./cmd/agent
	GOOS=linux   GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-arm64     ./cmd/agent
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64    ./cmd/agent
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64    ./cmd/agent
	GOOS=windows GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/agent

run:
	go run ./cmd/agent --config config.yaml

test-mode:
	go run ./cmd/agent --config config.yaml --test

once:
	go run ./cmd/agent --config config.yaml --once

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
