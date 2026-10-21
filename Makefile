-include .env

.PHONY: build test test-pg lint run-viewer run-http kitchen-sink db-up db-down db-reset db-psql

PG_ENV = FOLIO_DB_URL=$(FOLIO_DB_URL) FOLIO_HTTP_ADDR=$(FOLIO_HTTP_ADDR) FOLIO_PUBLIC_URL=$(FOLIO_PUBLIC_URL)

build:
	go build -o bin/folio ./cmd/folio

test:
	go test ./...

test-pg: db-up
	FOLIO_TEST_DB_URL=$(FOLIO_TEST_DB_URL) go test -count=1 ./...

lint:
	go vet ./...

run-viewer: build
	bin/folio serve

run-http: build db-up
	$(PG_ENV) bin/folio http

kitchen-sink:
	FOLIO_KITCHEN=$(CURDIR)/build/kitchen-sink go test ./internal/layout -run KitchenSink -count=1 -v | grep -E "pages|ok|FAIL"
	@echo "PDFs in build/kitchen-sink/"

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-reset:
	docker compose down -v
	docker compose up -d --wait

db-psql:
	docker compose exec db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)
