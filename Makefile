
all: build

clean:
	go clean -i ./...

vet:
	go vet ./...

test: vet
	go test -race -cover ./...

build: test
	go build ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fuzz:
	go test -run=^$$ -fuzz=FuzzSplit -fuzztime=30s
	go test -run=^$$ -fuzz=FuzzInstance -fuzztime=30s

update:
	go get -u ./...
