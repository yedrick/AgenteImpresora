//go:build windows && !embedagent

package main

// Sin la etiqueta "embedagent" no hay agente embebido: el instalador compila,
// pero avisa al ejecutarse de que hay que generarlo con BUILD_INSTALLER.bat.
var agentBinary []byte
