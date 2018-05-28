# Builds the review bot and its default plugins.
BIN      = bin/ReviewBot
SRC      = $(shell find src/rbbot/ -name '*.go')
MAIN_SRC = src/rbbot/main/main.go
GOPATH   = $(CURDIR)
GOBIN    = ${GOPATH}/bin
DB       = db/db.sqlite3
DB_SRC   = db/create-db.sql

${BIN} : ${SRC}
	env GOPATH=${GOPATH} GOBIN=${GOBIN} go install ${MAIN_SRC}

.PHONY: db

db: ${DB}

${DB}: ${DB_SRC}
	cat ${DB_SRC} | sqlite3 ${DB}

.PHONY: cleandb

cleandb:
	rm ${DB}

.PHONY: plugins

plugins:
	cd src/rbplugin/requester/httprequester && env GOPATH=${GOPATH} GOBIN=${GOBIN} $(MAKE)
	cp src/rbplugin/requester/httprequester/httprequester.so $(CURDIR)/plugins/request/
	cd src/rbplugin/reviewer/linereviewer && env GOPATH=${GOPATH} GOBIN=${GOBIN} $(MAKE)
	cp src/rbplugin/reviewer/linereviewer/linereviewer.so $(CURDIR)/plugins/review/
	cd src/rbplugin/reviewer/todoreviewer && env GOPATH=${GOPATH} GOBIN=${GOBIN} $(MAKE)
	cp src/rbplugin/reviewer/todoreviewer/todoreviewer.so $(CURDIR)/plugins/review/
