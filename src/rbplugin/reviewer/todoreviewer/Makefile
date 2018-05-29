NAME = todoreviewer
LIB  = todoreviewer.so
SRC  = todoreviewer.go

${LIB} : ${SRC}
	go build -ldflags "-pluginpath ${NAME}" -buildmode=plugin ${SRC}
