package main

import (
	"errors"
	"flag"
	"fmt"

	"dioptra/internal/roots"
	"dioptra/internal/store"
)

// runRoots gestiona el registro explícito de raíces de código (roots.json, junto a --dir).
//
//	dioptra roots list
//	dioptra roots set <proyecto> <ruta>
//	dioptra roots remove <proyecto>
func runRoots(args []string) error {
	fs := flag.NewFlagSet("roots", flag.ContinueOnError)
	dir := fs.String("dir", store.DefaultReportsDir(), "Directorio raíz de reportes Markdown (roots.json vive en su directorio padre)")
	// Reparseo para admitir flags en cualquier posición (mismo patrón que db_cmd.go).
	var pos []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(pos) == 0 {
		pos = []string{"list"}
	}
	reports := expandHomePath(*dir)
	reg, err := roots.Load(reports)
	if err != nil {
		return err
	}

	switch pos[0] {
	case "list":
		list := reg.List()
		fmt.Println("Registro:", reg.Path())
		if len(list) == 0 {
			fmt.Println("Sin raíces registradas. Usa: dioptra roots set <proyecto> <ruta>")
		}
		for _, e := range list {
			fmt.Printf("  %s → %s\n", e.Project, e.Path)
		}
	case "set":
		if len(pos) != 3 {
			return errors.New("uso: dioptra roots set <proyecto> <ruta>")
		}
		key, abs, err := reg.Set(pos[1], expandHomePath(pos[2]))
		if err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return err
		}
		fmt.Printf("Raíz registrada: %s → %s\n", key, abs)
	case "remove":
		if len(pos) != 2 {
			return errors.New("uso: dioptra roots remove <proyecto>")
		}
		ok := reg.Remove(pos[1])
		if ok {
			if err := reg.Save(); err != nil {
				return err
			}
		}
		fmt.Println("Raíz retirada:", ok)
	default:
		return fmt.Errorf("subcomando roots desconocido: %q (usa list|set|remove)", pos[0])
	}
	return nil
}
