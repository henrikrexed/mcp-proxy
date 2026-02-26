BINARY_NAME=mcp-otel-proxy
BUILD_DIR=bin
DOCKER_IMAGE=ghcr.io/isitobservable/mcp-otel-proxy
VERSION?=dev

.PHONY: build test lint run docker-build clean

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/mcp-otel-proxy

test:
	go test ./... -v

lint:
	golangci-lint run ./...

run: build
	UPSTREAM_URL=http://localhost:3000 \
	OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 \
	LOG_LEVEL=debug \
	$(BUILD_DIR)/$(BINARY_NAME)

docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

clean:
	rm -rf $(BUILD_DIR)
