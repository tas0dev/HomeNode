.PHONY: all web server agent test fmt clean
all: server agent
web:
	cd web && npm ci --ignore-scripts && npm run build
server: web
	mkdir -p build
	CGO_ENABLED=0 go build -mod=mod -trimpath -o build/homenode-server ./cmd/server
agent:
	mkdir -p build
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=mod -trimpath -o build/homenode-agent.exe ./cmd/agent
test: web
	cd web && npm test
	go test -mod=mod -race ./...
	go vet -mod=mod ./...
fmt:
	gofmt -w cmd internal web/embed.go
clean:
	rm -rf build
