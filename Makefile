.PHONY: build test lint run-viewer kitchen-sink

build:
	go build -o bin/folio ./cmd/folio

test:
	go test ./...

lint:
	go vet ./...

run-viewer: build
	bin/folio serve
