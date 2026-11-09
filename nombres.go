package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Los alias son locales: cambian el nombre visible en el panel y en print,
// nunca el nombre real de la cola en CUPS.

func rutaNombres() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".config", "print", "nombres.json")
}

func leerNombres() map[string]string {
	m := map[string]string{}
	p := rutaNombres()
	if p == "" {
		return m
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

// aliasDe devuelve el alias local de una cola ("" si no tiene).
func aliasDe(cola string) string {
	return strings.TrimSpace(leerNombres()[cola])
}

// GuardarNombresJSON aplica un mapa cola→nombre. Un valor vacío borra el alias
// de esa cola y la devuelve al nombre que declara CUPS. Los alias que no
// aparecen en el mapa se conservan.
func GuardarNombresJSON(texto string) error {
	var entrada map[string]string
	if err := json.Unmarshal([]byte(texto), &entrada); err != nil {
		return fmt.Errorf("lista de nombres no válida")
	}
	actual := leerNombres()
	for cola, nombre := range entrada {
		if !reColaValida.MatchString(cola) {
			return fmt.Errorf("nombre de cola no válido: %q", cola)
		}
		nombre = strings.TrimSpace(nombre)
		if nombre == "" {
			delete(actual, cola)
			continue
		}
		if len([]rune(nombre)) > 80 {
			return fmt.Errorf("el nombre de %s es demasiado largo (máximo 80)", cola)
		}
		if strings.ContainsAny(nombre, "\r\n\t") {
			return fmt.Errorf("el nombre de %s no puede tener saltos de línea", cola)
		}
		actual[cola] = nombre
	}
	p := rutaNombres()
	if p == "" {
		return fmt.Errorf("no encuentro el directorio personal")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
