//go:build windows

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// HKCU no requiere permisos de administrador y solo afecta a la cuenta actual.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func writeAutostartEntry(cmdline string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("abriendo HKCU\\%s: %w", runKeyPath, err)
	}
	defer k.Close()
	if err := k.SetStringValue(autostartValueName, cmdline); err != nil {
		return fmt.Errorf("guardando la entrada de arranque: %w", err)
	}
	return nil
}

func readAutostartEntry() (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("abriendo HKCU\\%s: %w", runKeyPath, err)
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("leyendo la entrada de arranque: %w", err)
	}
	return v, true, nil
}

func removeAutostartEntry() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("abriendo HKCU\\%s: %w", runKeyPath, err)
	}
	defer k.Close()
	err = k.DeleteValue(autostartValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("borrando la entrada de arranque: %w", err)
	}
	return true, nil
}
