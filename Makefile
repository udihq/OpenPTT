BINARY  := bridge
VERSION ?= v1.1.0

.PHONY: all tidy libdave build run test clean docker

all: build

tidy:
	go mod tidy

# Build and install Discord's libdave, which godave/golibdave links against.
libdave:
	./scripts/install-libdave.sh

build: tidy
	CGO_ENABLED=1 go build -o $(BINARY) -ldflags "-X main.version=$(VERSION)" ./cmd/bridge

run: build
	./$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY)

docker:
	docker compose build
