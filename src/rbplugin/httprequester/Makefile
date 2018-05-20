NAME = httprequester
LIB  = httprequester.so
SRC  = httprequester.go

${LIB} : ${SRC}
	go build -ldflags "-pluginpath ${NAME}" -buildmode=plugin ${SRC}
