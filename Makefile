.PHONY: dev build web test clean

dev:
	go run ./cmd/teleflow

web:
	cd web && npm ci && npm run build

build: web
	go build -trimpath -o dist/teleflow ./cmd/teleflow

test:
	go test ./cmd/... ./internal/...
	cd web && npm run typecheck

clean:
	rm -rf dist web/node_modules
