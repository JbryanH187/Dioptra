//go:build !windows

package main

func writeAutostartEntry(string) error { return errAutostartUnsupported }

func readAutostartEntry() (string, bool, error) { return "", false, errAutostartUnsupported }

func removeAutostartEntry() (bool, error) { return false, errAutostartUnsupported }
