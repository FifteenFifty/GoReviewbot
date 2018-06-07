# Builds the review bot and its default plugins.
BIN       = bin/ReviewBot
SRC       = $(shell find src/rbbot/ -name '*.go')
MAIN_SRC  = src/rbbot/main/main.go
GOPATH    = $(CURDIR)
GOBIN     = ${GOPATH}/bin
DB        = db/db.sqlite3
DB_SRC    = db/create-db.sql
PLUGINDIR = $(CURDIR)/plugins

REQPLUGIN_SRC  = $(shell ls src/rbplugin/requester)
REQPLUGIN_OUT  = $(addprefix ${PLUGINDIR}/request/, $(addsuffix .so,${REQPLUGIN_SRC}))
REVPLUGIN_SRC  = $(shell ls src/rbplugin/reviewer)
REVPLUGIN_OUT  = $(addprefix ${PLUGINDIR}/review/, $(addsuffix .so,${REVPLUGIN_SRC}))

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

plugins: ${PLUGINDIR}/review/ ${PLUGINDIR}/request/ ${REQPLUGIN_OUT} ${REVPLUGIN_OUT}

${PLUGINDIR}/review/%.so: $(CURDIR)/src/rbplugin/reviewer/%
	env GOPATH=${GOPATH} GOBIN=${GOBIN} go build -ldflags "-pluginpath ${plugindir}" -buildmode=plugin $</$(basename $(notdir $<)).go

${PLUGINDIR}/request/%.so: $(CURDIR)/src/rbplugin/requester/%
	env GOPATH=${GOPATH} GOBIN=${GOBIN} go build -ldflags "-pluginpath ${plugindir}" -buildmode=plugin $</$(basename $(notdir $<)).go


${PLUGINDIR}/request:
	mkdir -p $@

${PLUGINDIR}/review:
	mkdir -p $@
