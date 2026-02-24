# Makefile for goModbus

BINARY_NAME=modbus_client
CONFIG_FILE=config.yaml

.PHONY: all build run clean deps ui

all: build

build:
	go build -o $(BINARY_NAME).exe .

run: build
	./$(BINARY_NAME).exe

ui: build
	@echo "🚀 UI available at http://localhost:8080"
	./$(BINARY_NAME).exe

clean:
	if [ -f $(BINARY_NAME).exe ]; then rm $(BINARY_NAME).exe; fi

deps:
	go mod tidy
