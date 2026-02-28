.PHONY: all build build-mock test clean

all: build

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/localrouter ./cmd/server/main.go

build-mock:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/localrouter-mock ./cmd/mock/main.go

test:
	go test -v -race ./...

clean:
	rm -rf bin/
