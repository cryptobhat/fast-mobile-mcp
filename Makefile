.PHONY: proto build-gateway build-workers mcp-stdio e2e-smoke

proto:
	./scripts/gen-proto.sh

build-gateway:
	cd gateway-mcp && npm run build

build-workers:
	cd worker-android && go build ./cmd/worker
	cd worker-ios && go build ./cmd/worker

mcp-stdio:
	cd gateway-mcp && npm run mcp:stdio

e2e-smoke:
	cd gateway-mcp && npm run e2e:smoke