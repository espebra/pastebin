deps:
	go get -u github.com/jteeuwen/go-bindata/...
	go get github.com/elazarl/go-bindata-assetfs/...

build:
	go-bindata-assetfs static/... templates/...
	mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build -o bin/pastebin-darwin-amd64
	GOOS=linux GOARCH=amd64 go build -o bin/pastebin-linux-amd64

install:
	go-bindata-assetfs static/... templates/...
	go install
