NAME = linereviewer
LIB  = linereviewer.so
SRC  = linereviewer.go

${LIB} : ${SRC}
	go build -ldflags "-pluginpath ${NAME}" -buildmode=plugin ${SRC}
