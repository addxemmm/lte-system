.PHONY: all build test vet clean docker

all: build

build:
	go build -o bin/lte-system ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin

# Cross-compile for the Ubuntu server (amd64 linux) from Windows/macOS.
linux:
	GOOS=linux GOARCH=amd64 go build -o bin/lte-system-linux-amd64 ./cmd/server

docker:
	docker build -f deploy/docker/Dockerfile -t ltesystem-dep:2.0 .
	docker tag ltesystem-dep:2.0 docker.skygo/addx/ltesystem-dep:2.0
