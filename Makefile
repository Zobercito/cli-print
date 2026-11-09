BIN     ?= print
PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin

.PHONY: all build install uninstall test vet fmt clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) .

install: build
	install -Dm755 $(BIN) $(BINDIR)/$(BIN)

uninstall:
	rm -f $(BINDIR)/$(BIN)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f $(BIN)
