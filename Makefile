BINARY := pamie
MAIN := ./cmd/pamie
PKG := ./...
VERSION ?= dev
IMAGE ?= kurocho/pamie
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: fmt test vet build run docker-build docker-smoke docker-push release-snapshot checksums ci

fmt:
	gofmt -w ./cmd ./internal

test:
	go test $(PKG)

vet:
	go vet $(PKG)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY) $(MAIN)

run:
	go run $(MAIN)

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

docker-smoke:
	PAMIE_IMAGE=$(IMAGE):smoke ./scripts/docker-smoke.sh

docker-push:
	docker push $(IMAGE):$(VERSION)

release-snapshot:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_linux_amd64 $(MAIN)
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_linux_arm64 $(MAIN)
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_darwin_amd64 $(MAIN)
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_darwin_arm64 $(MAIN)

checksums:
	cd dist && shasum -a 256 * > checksums.txt

ci: fmt test vet build
