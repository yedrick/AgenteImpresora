//go:build windows && embedagent

package main

import _ "embed"

// El agente se embebe solo con la etiqueta "embedagent", que pone
// BUILD_INSTALLER.bat despues de compilar CollaTechAgent.bin. Asi
// "go build ./..." y "go test ./..." funcionan en un clon limpio, sin tener
// que versionar 8 MB de binario ni un marcador vacio.
//
//go:embed CollaTechAgent.bin
var agentBinary []byte
