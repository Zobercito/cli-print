package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const opcionesFalsas = "printer-info='Canon PIXMA G2110' printer-make-and-model='Canon PIXMA G2110' printer-location='Estudio' PageSize=Letter ColorModel=RGB Duplex=None print-quality=4"

func TestDetalles(t *testing.T) {
	conFalsos(t, "parada")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PRINT_SYSUSB", t.TempDir())
	t.Setenv("FAKE_URI", "usb://Canon/G2110?serial=NOEXISTE")
	t.Setenv("FAKE_LPOPTIONS", opcionesFalsas)

	d := LeerDetalles(context.Background(), "Canon_G2110")
	if !d.Cups {
		t.Fatal("CUPS debería responder")
	}
	if d.URI != "usb://Canon/G2110?serial=NOEXISTE" {
		t.Errorf("URI: %q", d.URI)
	}
	if d.Modelo != "Canon PIXMA G2110" || d.Ubicacion != "Estudio" {
		t.Errorf("modelo/ubicación: %+v", d)
	}
	if d.Nombre != "Canon PIXMA G2110" {
		t.Errorf("nombre: %q", d.Nombre)
	}
	if d.Estado != "desconectada" {
		t.Errorf("estado: %q", d.Estado)
	}
	if !d.Predeterminada {
		t.Error("debería ser la predeterminada")
	}
	if d.Opciones["PageSize"] != "Letter" || d.Opciones["ColorModel"] != "RGB" {
		t.Errorf("opciones: %+v", d.Opciones)
	}
	if d.Habilitada || d.Aceptando {
		t.Errorf("la cola falsa está parada: %+v", d)
	}
}

func TestImprimirPrueba(t *testing.T) {
	log := conFalsos(t, "")
	dir := t.TempDir()
	pagina := filepath.Join(dir, "testprint")
	if err := os.WriteFile(pagina, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	viejo := paginaPrueba
	paginaPrueba = pagina
	t.Cleanup(func() { paginaPrueba = viejo })

	id, err := ImprimirPrueba(context.Background(), "Canon_G2110")
	if err != nil || id != "Canon_G2110-42" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-d Canon_G2110 "+pagina) {
		t.Errorf("log: %s", b)
	}
}

func TestCLIDetalles(t *testing.T) {
	conFalsos(t, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FAKE_URI", "ipp://localhost/printers/Canon_G2110")
	t.Setenv("FAKE_LPOPTIONS", opcionesFalsas)

	var salida strings.Builder
	ok, code := subcomando(context.Background(), []string{"detalles", "--json"},
		Config{Impresora: "Canon_G2110"}, &salida, &salida)
	if !ok || code != 0 {
		t.Fatalf("ok=%v code=%d salida=%s", ok, code, salida.String())
	}
	var r Resultado
	if err := json.Unmarshal([]byte(salida.String()), &r); err != nil {
		t.Fatalf("json: %v (%s)", err, salida.String())
	}
	if r.Detalles == nil || r.Detalles.Cola != "Canon_G2110" || !r.Detalles.Cups {
		t.Fatalf("resultado: %+v", r)
	}
	if r.Detalles.Opciones["print-quality"] != "4" {
		t.Errorf("opciones: %+v", r.Detalles.Opciones)
	}
}
