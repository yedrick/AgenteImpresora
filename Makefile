# CollaTech Agent - construccion para todas las plataformas.
#
# Hasta ahora solo habia scripts .bat, asi que fuera de Windows no habia forma
# de construir el producto aunque el codigo compile para seis destinos. Y el
# .bat no sellaba la version: el binario respondia "dev" a --version.

VERSION := 1.5.0
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist

# El binario del host, el que se usa para probar en esta misma maquina.
BIN := collatech-agent

.PHONY: all build run test check fmt vet dist clean install uninstall sdk version-check help

help:
	@echo "CollaTech Agent $(VERSION)"
	@echo
	@echo "  make build        compila el agente para esta maquina"
	@echo "  make run          lo arranca aqui mismo (Ctrl-C para parar)"
	@echo "  make test         pruebas con detector de carreras"
	@echo "  make check        formato + vet + pruebas + version"
	@echo "  make dist         compila los seis destinos en $(DIST)/"
	@echo "  make sdk          reconstruye y empaqueta el SDK de TypeScript"
	@echo "  make install      lo instala como servicio del sistema (pide sudo)"
	@echo "  make uninstall    lo quita"
	@echo "  make clean        borra lo construido"

all: check dist

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server
	@echo "listo: ./$(BIN) $$(./$(BIN) --version | awk '{print $$3}')"

run: build
	./$(BIN)

test:
	go test ./... -race

fmt:
	@test -z "$$(gofmt -l . | grep -v node_modules)" || { echo "sin formatear:"; gofmt -l . | grep -v node_modules; exit 1; }
	@echo "formato correcto"

vet:
	go vet ./...

# La version vive en dos sitios que nadie ata: aqui y en el SDK. Si se
# separan, el cliente recibe un agente y un SDK que dicen cosas distintas.
version-check:
	@sdk=$$(grep -o '"version": *"[^"]*"' collatech-sdk/package.json | head -1 | cut -d'"' -f4); \
	if [ "$$sdk" != "$(VERSION)" ]; then \
		echo "la version del SDK ($$sdk) no coincide con la del agente ($(VERSION))"; exit 1; \
	fi; \
	echo "version $(VERSION) en el agente y en el SDK"

# La guia de Ubuntu se pinta en el navegador con un conversor propio; esto
# comprueba que sigue produciendo el HTML esperado.
guia:
	@node examples/comprobar-guia.js

check: fmt vet version-check test guia
	@echo "todo en orden"

# Los seis destinos que el codigo soporta. El ejecutable de Windows va sin
# -H windowsgui a proposito: con esa bandera, 'CollaTechAgent.exe --install'
# no imprime nada en la consola y parece que no hizo nada.
dist:
	@mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/collatech-agent-linux-amd64        ./cmd/server
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/collatech-agent-linux-arm64        ./cmd/server
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/collatech-agent-macos-intel        ./cmd/server
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/collatech-agent-macos-apple        ./cmd/server
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/CollaTechAgent-windows-amd64.exe   ./cmd/server
	GOOS=windows GOARCH=386   go build -ldflags "$(LDFLAGS)" -o $(DIST)/CollaTechAgent-windows-386.exe     ./cmd/server
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/INSTALADOR-windows-amd64.exe       ./cmd/installer
	@echo
	@ls -lh $(DIST)/ | tail -n +2 | awk '{printf "  %-42s %s\n", $$9, $$5}'

sdk:
	cd collatech-sdk && npm run build && rm -f collatech-sdk-*.tgz && npm pack
	@# El agente sirve el SDK desde dentro del binario, para las cajas sin
	@# internet. Si no se copia aqui, se entrega una version vieja.
	cp collatech-sdk/collatech-sdk-$(VERSION).tgz internal/api/web/sdk/collatech-sdk.tgz
	@echo "empaquetado y embebido: collatech-sdk-$(VERSION).tgz"

install: build
	sudo ./$(BIN) --install

uninstall:
	sudo ./$(BIN) --uninstall

clean:
	rm -rf $(DIST) $(BIN) CollaTechAgent.exe INSTALADOR.exe
	go clean -testcache
