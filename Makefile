# Builds the review bot. Note that this only builds the main project, not any
# plugins
BIN = bin/ReviewBot
SRC = $(shell find src/rbbot/ -name '*.go')
MAIN_SRC = src/rbbot/main/main.go

${BIN} : ${SRC}
	env GOPATH=$(CURDIR) GOBIN=$(CURDIR)/bin/ go install ${MAIN_SRC}

