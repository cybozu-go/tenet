BIN_DIR := $(shell pwd)/bin

# Tool versions
MDBOOK_VERSION = 0.4.10
MDBOOK := $(BIN_DIR)/mdbook

# Test tools
STATICCHECK = $(BIN_DIR)/staticcheck
NILERR = $(BIN_DIR)/nilerr

.PHONY: all
all: test

.PHONY: book
book: $(MDBOOK)
	rm -rf docs/book
	cd docs; $(MDBOOK) build


.PHONY: test
test: test-tools
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	$(STATICCHECK) ./...
	test -z "$$($(NILERR) ./... 2>&1 | tee /dev/stderr)"
	go install ./...
	go test -race -v ./...
	go vet ./...


##@ Tools

$(MDBOOK):
	mkdir -p bin
	curl -fsL https://github.com/rust-lang/mdBook/releases/download/v$(MDBOOK_VERSION)/mdbook-v$(MDBOOK_VERSION)-x86_64-unknown-linux-gnu.tar.gz | tar -C bin -xzf -

.PHONY: test-tools
test-tools: $(STATICCHECK) $(NILERR)

$(STATICCHECK):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install honnef.co/go/tools/cmd/staticcheck@latest

$(NILERR):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/gostaticanalysis/nilerr/cmd/nilerr@latest
