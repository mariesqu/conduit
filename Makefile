.PHONY: build dev clean vendor tidy

BIN := conduit.exe
GO  := go
LDFLAGS := -s -w

build:
	cd ui && npm install --no-audit --no-fund && npm run build
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

dev:
	cd ui && npm install --no-audit --no-fund
	@echo "Run these in two terminals:"
	@echo "  (1) cd ui && npm run dev"
	@echo "  (2) go run ."

tidy:
	$(GO) mod tidy

vendor: tidy
	$(GO) mod vendor

clean:
	rm -f $(BIN)
	rm -rf ui/dist ui/node_modules
