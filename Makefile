.PHONY: build test release
build:
	go build -o bin/lazyiperf ./cmd/lazyiperf
test:
	go test -race ./...
release:
	./scripts/release.sh $(VERSION)
