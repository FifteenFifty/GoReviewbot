# Builds the review bot and its default plugins.
BIN      = bin/ReviewBot
SRC      = $(shell find src/rbbot/ -name '*.go')
MAIN_SRC = src/rbbot/main/main.go
GOPATH   = $(CURDIR)
GOBIN    = ${GOPATH}/bin

${BIN} : ${SRC}
	env GOPATH=${GOPATH} GOBIN=${GOBIN} go install ${MAIN_SRC}

.PHONY: test

test:
	cd src/rbplugin/httprequester && env GOPATH=${GOPATH} GOBIN=${GOBIN} $(MAKE)
