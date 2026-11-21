package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dioptra/internal/store"
)

// autostartValueName es el nombre de la entrada en HKCU\...\Run (Windows).
const autostartValueName = "DioptraEngine"

var errAutostartUnsupported = errors.New("el arranque automático solo está disponible en Windows por ahora")

// runStart deja el demonio `dioptra serve` corriendo en segundo plano y termina.
// Si ya responde, no hace nada. Es lo que ejecuta el arranque automático al iniciar sesión.
func runStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto TCP loopback (127.0.0.1)")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ensureLocalDaemon(ctx, *port, expandHomePath(*dir)); err != nil {
		return err
	}
	fmt.Printf("dioptra serve activo en http://127.0.0.1:%d\n", *port)
	return nil
}

// runAutostart: dioptra autostart on|off|status [--port] [--dir]
func runAutostart(args []string) error {
	action := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = strings.ToLower(strings.TrimSpace(args[0]))
		args = args[1:]
	}
	fs := flag.NewFlagSet("autostart", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "Puerto TCP loopback (127.0.0.1)")
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch action {
	case "on", "enable":
		exe, err := currentExecutable()
		if err != nil {
			return err
		}
		reportsDir, err := filepath.Abs(expandHomePath(*dir))
		if err != nil {
			return err
		}
		cmdline := autostartCommandLine(conhostPath(), exe, *port, reportsDir)
		if err := writeAutostartEntry(cmdline); err != nil {
			return err
		}
		fmt.Println("Arranque automático activado: Dioptra se levantará en segundo plano al iniciar sesión en Windows.")
		fmt.Printf("  %s\n", cmdline)
		fmt.Println("Para quitarlo: dioptra autostart off")
		return nil
	case "off", "disable":
		removed, err := removeAutostartEntry()
		if err != nil {
			return err
		}
		if removed {
			fmt.Println("Arranque automático desactivado.")
		} else {
			fmt.Println("El arranque automático ya estaba desactivado.")
		}
		return nil
	case "status":
		cmdline, found, err := readAutostartEntry()
		if err != nil {
			return err
		}
		if !found {
			fmt.Println("Arranque automático: desactivado (actívalo con: dioptra autostart on)")
			return nil
		}
		fmt.Println("Arranque automático: activado")
		fmt.Printf("  %s\n", cmdline)
		if exe := autostartExecutable(cmdline); exe != "" {
			if _, err := os.Stat(exe); err != nil {
				fmt.Printf("  ⚠ el binario ya no existe en esa ruta; vuelve a ejecutar «dioptra autostart on» desde el nuevo\n")
			}
		}
		return nil
	default:
		return fmt.Errorf("acción desconocida %q: usa on, off o status", action)
	}
}

// autostartCommandLine arma la línea que Windows ejecuta al iniciar sesión.
// conhost --headless evita que se abra una ventana de consola; `start` solo
// lanza el demonio desacoplado y termina.
func autostartCommandLine(conhost, exe string, port int, reportsDir string) string {
	return fmt.Sprintf(`%s --headless %s start --port %d --dir %s`,
		quoteWinArg(conhost), quoteWinArg(exe), port, quoteWinArg(reportsDir))
}

// autostartExecutable recupera la ruta de dioptra.exe de una línea guardada.
func autostartExecutable(cmdline string) string {
	parts := strings.Split(cmdline, `"`)
	// Formato: "conhost" --headless "dioptra" start ... → parts[3] es dioptra.exe
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

// quoteWinArg entrecomilla siempre: las rutas de OneDrive suelen tener espacios.
// Una barra final se duplica para que no escape la comilla de cierre.
func quoteWinArg(s string) string {
	if strings.HasSuffix(s, `\`) {
		s += `\`
	}
	return `"` + s + `"`
}

func conhostPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "conhost.exe")
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolviendo ruta del binario dioptra: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}
