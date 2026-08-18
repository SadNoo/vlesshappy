.PHONY: fmt test race vet build verify

fmt:
	gofmt -w cmd internal

test:
	go test -mod=vendor ./...

race:
	go test -mod=vendor -race ./...

vet:
	go vet -mod=vendor ./...

build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -ldflags="-s -w -buildid=" -o dist/vlesshappy-linux-amd64 ./cmd/vlesshappy

verify: test race vet build
