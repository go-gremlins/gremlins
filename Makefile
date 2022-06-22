GO_CMD=go
GO_TEST=${GO_CMD} test
GO_BUILD=${GO_CMD} build
BINARY_NAME=gremlins
TARGET=out
BIN=${TARGET}/bin

all: lint test build

build:
	mkdir -p ${BIN}
	${GO_BUILD} -o ${BIN}/${BINARY_NAME} cmd/gremlins/main.go

test:
	${GO_TEST} ./...

lint:
	golangci-lint run ./...

clean:
	go clean
	rm -rf ${TARGET}