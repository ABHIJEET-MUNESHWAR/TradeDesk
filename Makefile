BINARY := tradedesk
PKG := ./...

.PHONY: all build run test race cover bench fmt vet tidy lint docker clean

all: fmt vet test

build:
	go build -trimpath -o bin/$(BINARY) ./cmd/$(BINARY)

run:
	go run ./cmd/$(BINARY)

test:
	go test $(PKG)

race:
	go test -race $(PKG)

cover:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -1

bench:
	go test -bench=. -benchmem ./internal/domain/

fmt:
	gofmt -s -w internal cmd

vet:
	go vet $(PKG)

tidy:
	go mod tidy

lint:
	golangci-lint run

docker:
	docker build -t $(BINARY):latest .

clean:
	rm -rf bin coverage.out
