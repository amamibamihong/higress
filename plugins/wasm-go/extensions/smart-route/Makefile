.PHONY: build clean

PLUGIN_NAME = smart-route

build:
	go mod tidy
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./main.go

clean:
	rm -f main.wasm
