APP=psp-health-service

API_BASE_URL ?= http://localhost:8080
WS_DEMO_URL ?= ws://localhost:8080/events/stream
WS_STREAM_TOKEN ?=

.PHONY: run test test-race generate-data ws-demo ws-demo-degraded demo-degraded-now fmt verify frontend-install frontend-dev frontend-build frontend-start

run:
	go run ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

generate-data:
	go run ./cmd/generate-testdata -out testdata/transactions.json -count 800

ws-demo:
	go run ./cmd/ws-demo -url "$(WS_DEMO_URL)" $(if $(WS_STREAM_TOKEN),-token "$(WS_STREAM_TOKEN)")

ws-demo-degraded:
	go run ./cmd/ws-demo -url "$(WS_DEMO_URL)" -scenario degraded -psp PSP_GAMMA -batch $(if $(WS_STREAM_TOKEN),-token "$(WS_STREAM_TOKEN)")

demo-degraded-now:
	go run ./cmd/ws-demo -url "$(WS_DEMO_URL)" -scenario degraded -psp PSP_GAMMA -batch $(if $(WS_STREAM_TOKEN),-token "$(WS_STREAM_TOKEN)")
	@echo "---- PSP_GAMMA health snapshot ----"
	@curl -sS "$(API_BASE_URL)/health?psp=PSP_GAMMA"
	@echo
	@echo "---- PSP_GAMMA active alerts ----"
	@curl -sS "$(API_BASE_URL)/alerts?psp=PSP_GAMMA&active_only=true"
	@echo

fmt:
	gofmt -w ./cmd ./internal

verify: fmt test test-race

frontend-install:
	cd frontend && npm install

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

frontend-start:
	cd frontend && npm run start
