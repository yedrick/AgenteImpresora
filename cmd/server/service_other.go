//go:build !windows

package main

// runService no hace nada fuera de Windows: no hay Service Control Manager,
// asi que el agente siempre corre en modo interactivo.
func runService() (bool, error) { return false, nil }
