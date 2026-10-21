.PHONY: build test lint run-viewer kitchen-sink

build:
	go build -o bin/folio ./cmd/folio

test:
	go test ./...

lint:
	go vet ./...

run-viewer: build
	bin/folio serve

kitchen-sink:
	FOLIO_KITCHEN=$(CURDIR)/build/kitchen-sink go test ./internal/layout -run KitchenSink -count=1 -v | grep -E "pages|ok|FAIL"
	@echo "PDFs in build/kitchen-sink/"
