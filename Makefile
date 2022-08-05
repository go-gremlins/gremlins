GO_CMD=go
GO_TEST=${GO_CMD} test
GO_BUILD=${GO_CMD} build
RELEASER_CMD=goreleaser
RELEASE=${RELEASER_CMD} release
LINTER_CMD=golangci-lint
LINT=${LINTER_CMD} run
BINARY_NAME=gremlins
COVER_OUT=coverage.out
TARGET=dist
BIN=${TARGET}/bin

.PHONY: all snap
all: lint test build
snap: lint test snapshot

build: ${BIN}/${BINARY_NAME}

${BIN}/${BINARY_NAME}:
	mkdir -p ${BIN}
	${GO_BUILD} -o ${BIN}/${BINARY_NAME} cmd/gremlins/main.go

.PHONY: test
test:
	${GO_TEST} -race ./...

.PHONY: cover
cover: ${COVER_OUT}

${COVER_OUT}:
	${GO_TEST} -race -covermode=atomic -cover -coverprofile ${COVER_OUT} ./...

.PHONY: requirements
requirements:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest

.PHONY: lint
lint:
	${LINT} ./...

.PHONY: goimports
goimports:
	goimports --local 'github.com/go-gremlins/gremlins' -v -w .

.PHONY: fieldalignment
fieldalignment:
	fieldalignment -fix ./...

.PHONY: snapshot
snapshot:
	${RELEASE} --snapshot --rm-dist

.PHONY: clean
clean:
	go clean
	rm -rf -- ${TARGET}
	rm -f -- coverage.out