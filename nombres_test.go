package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAliasLocal(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	ctx := context.Background()

	if err := GuardarNombresJSON(`{"Canon_G2110":"Mi Canon"}`); err != nil {
		t.Fatal(err)
	}
	if n := nombreImpresora(ctx, "Canon_G2110"); n != "Mi Canon" {
		t.Errorf("alias: %q", n)
	}
	if b, err := os.ReadFile(filepath.Join(h, ".config", "print", "nombres.json")); err != nil || len(b) == 0 {
		t.Fatalf("archivo de nombres: %v", err)
	}

	// Los alias que no aparecen en el mapa se conservan.
	if err := GuardarNombresJSON(`{"Otra_Cola":"Otra"}`); err != nil {
		t.Fatal(err)
	}
	if n := nombreImpresora(ctx, "Canon_G2110"); n != "Mi Canon" {
		t.Errorf("alias perdido: %q", n)
	}

	// Un nombre vacío borra el alias y vuelve al nombre de CUPS.
	if err := GuardarNombresJSON(`{"Canon_G2110":""}`); err != nil {
		t.Fatal(err)
	}
	if n := nombreImpresora(ctx, "Canon_G2110"); n == "Mi Canon" {
		t.Error("el alias vacío debería borrarlo")
	}

	if err := GuardarNombresJSON(`{"cola rara":"x"}`); err == nil {
		t.Error("debería rechazar una cola inválida")
	}
	if err := GuardarNombresJSON(`no json`); err == nil {
		t.Error("debería rechazar JSON inválido")
	}
}
