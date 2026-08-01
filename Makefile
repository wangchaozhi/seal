.PHONY: test dev-api test-api fmt-api dev-web build-web dev-worker test-worker

test: test-api test-worker build-web

dev-api:
	cd apps/api && go run ./cmd/server

test-api:
	cd apps/api && go test ./...

fmt-api:
	cd apps/api && gofmt -w ./cmd ./internal

dev-web:
	cd apps/web && npm run dev

build-web:
	cd apps/web && npm run build

dev-worker:
	cd apps/worker && npm start

test-worker:
	cd apps/worker && npm test
