BINARY     := duckflux
BUILD_DIR  := bin
CMD_DIR    := ./cmd/duckflux

.PHONY: all build test lint clean

all: build

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)
