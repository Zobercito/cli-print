package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// entornoCups prepara CUPS falso: cola con los trabajos dados (líneas CSV de
// ipptool), USB presente y estado en un directorio temporal.
func configPrueba() Config {
	return Config{Impresora: "Canon_G2110", Tinta: "color", Animacion: true, Giro: "libro", TimeoutSoffice: 60}
}

func entornoCups(t *testing.T, usb bool, parada bool, trabajos ...string) (log, jobs string) {
	t.Helper()
	log = conFalsos(t, "")
	d := t.TempDir()
	jobs = filepath.Join(d, "jobs.csv")
	os.WriteFile(jobs, []byte(strings.Join(trabajos, "\n")+"\n"), 0o644)
	if len(trabajos) == 0 {
		os.WriteFile(jobs, nil, 0o644)
	}
	t.Setenv("FAKE_JOBS", jobs)
	t.Setenv("FAKE_PARADA_FILE", "")
	t.Setenv("FAKE_ALERTS", "")
	if parada {
		pf := filepath.Join(d, "parada")
		os.WriteFile(pf, nil, 0o644)
		t.Setenv("FAKE_PARADA_FILE", pf)
	}
	usbDir := filepath.Join(d, "usb")
	if usb {
		os.MkdirAll(filepath.Join(usbDir, "1-1"), 0o755)
		os.WriteFile(filepath.Join(usbDir, "1-1", "idVendor"), []byte("1234\n"), 0o644)
		os.WriteFile(filepath.Join(usbDir, "1-1", "idProduct"), []byte("5678\n"), 0o644)
		os.WriteFile(filepath.Join(usbDir, "1-1", "manufacturer"), []byte("Ejemplo\n"), 0o644)
		os.WriteFile(filepath.Join(usbDir, "1-1", "product"), []byte("Modelo series\n"), 0o644)
		os.WriteFile(filepath.Join(usbDir, "1-1", "serial"), []byte("SERIE123\n"), 0o644)
		t.Setenv("FAKE_URI", "usb://Ejemplo/Modelo%20series?serial=SERIE123&interface=1")
	} else {
		os.MkdirAll(usbDir, 0o755)
		t.Setenv("FAKE_URI", "usb://Ejemplo/Modelo%20series?serial=NOEXISTE&interface=1")
	}
	t.Setenv("PRINT_SYSUSB", usbDir)
	t.Setenv("PRINT_ESTADO", filepath.Join(d, "estado"))
	return log, jobs
}

func ahoraMenos(s int) int64 { return time.Now().Unix() - int64(s) }

func linea(id int, nombre, estado, usuario string, hace int) string {
	return fmt.Sprintf("%d,%q,%s,none,%s,26,%d", id, nombre, estado, usuario, ahoraMenos(hace))
}

func TestEstadoLista(t *testing.T) {
	entornoCups(t, true, false)
	e := LeerEstado(context.Background(), "Canon_G2110")
	if e.Estado != "lista" || !e.USB || !e.Habilitada || len(e.Trabajos) != 0 || e.V != 1 {
		t.Errorf("%+v", e)
	}
	if b, _ := json.Marshal(e); !strings.Contains(string(b), `"tinta":null`) {
		t.Errorf("tinta debe ser null: %s", b)
	}
}

func TestEstadoEsperaConNombresYOrigen(t *testing.T) {
	entornoCups(t, true, false,
		linea(25, "informe, versión final.pdf", "pending", miUsuario(), 3600),
		linea(26, "guía.docx", "pending-held", miUsuario(), 60),
		linea(27, "ajeno.pdf", "pending", "otrouser", 10))
	RegistrarEnvio("Canon_G2110-26", "guía.docx")
	e := LeerEstado(context.Background(), "Canon_G2110")
	if e.Estado != "espera" || len(e.Trabajos) != 3 {
		t.Fatalf("%+v", e)
	}
	t0, t1, t2 := e.Trabajos[0], e.Trabajos[1], e.Trabajos[2]
	if t0.Nombre != "informe, versión final.pdf" || t0.Estado != "pendiente" || !t0.Propio || t0.Origen != "otro" {
		t.Errorf("t0 %+v", t0)
	}
	if t1.Estado != "retenido" || t1.Origen != "print" {
		t.Errorf("t1 %+v", t1)
	}
	if t2.Propio {
		t.Errorf("el trabajo de otro usuario no es propio: %+v", t2)
	}
	if t0.EdadS < 3500 || t0.EdadS > 3700 {
		t.Errorf("edad %d", t0.EdadS)
	}
	hay := false
	for _, a := range e.Alertas {
		if a.Codigo == "J-1" {
			hay = true
		}
	}
	if !hay {
		t.Error("falta el aviso de trabajo viejo")
	}
}

func TestEstadosDetenidaDesconectadaError(t *testing.T) {
	entornoCups(t, true, true, linea(25, "a.pdf", "pending", miUsuario(), 100))
	if e := LeerEstado(context.Background(), "Canon_G2110"); e.Estado != "detenida" || e.Habilitada {
		t.Errorf("detenida: %+v", e)
	}
	entornoCups(t, false, true, linea(25, "a.pdf", "pending", miUsuario(), 100))
	if e := LeerEstado(context.Background(), "Canon_G2110"); e.Estado != "desconectada" || len(e.Trabajos) != 1 {
		t.Errorf("desconectada: %+v", e)
	}
	entornoCups(t, true, false)
	t.Setenv("FAKE_ALERTS", "media-empty")
	if e := LeerEstado(context.Background(), "Canon_G2110"); e.Estado != "error" || e.Alertas[0].Texto != "Sin papel" {
		t.Errorf("error: %+v", e)
	}
	t.Setenv("FAKE_ALERTS", "marker-supply-low-warning")
	if e := LeerEstado(context.Background(), "Canon_G2110"); e.Estado != "lista" || e.Alertas[0].Texto != "Tinta baja" {
		t.Errorf("tinta baja no bloquea: %+v", e)
	}
}

func TestEstadoSinCupsYSinCola(t *testing.T) {
	entornoCups(t, true, false)
	if e := LeerEstado(context.Background(), "NoExiste"); e.Estado != "sin_cola" {
		t.Errorf("%+v", e)
	}
	t.Setenv("PATH", t.TempDir()) // sin lpstat
	if e := LeerEstado(context.Background(), "Canon_G2110"); e.Estado != "sin_cups" {
		t.Errorf("%+v", e)
	}
}

func TestTrabajosSinIpptoolCaeALpstat(t *testing.T) {
	entornoCups(t, true, false, linea(25, "a.pdf", "pending", miUsuario(), 100))
	t.Setenv("FAKE_IPPTOOL", "falla")
	e := LeerEstado(context.Background(), "Canon_G2110")
	if len(e.Trabajos) != 1 || e.Trabajos[0].ID != "Canon_G2110-25" || e.Trabajos[0].Nombre != "Canon_G2110-25" {
		t.Errorf("%+v", e.Trabajos)
	}
}

func TestCancelar(t *testing.T) {
	log, jobs := entornoCups(t, true, false, linea(25, "a.pdf", "pending", miUsuario(), 100))
	if err := Cancelar(context.Background(), "Canon_G2110-25"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(jobs); len(bytes.TrimSpace(b)) != 0 {
		t.Errorf("el trabajo debía desaparecer: %s", b)
	}
	if err := Cancelar(context.Background(), "Canon_G2110-25"); err != errYaNoExiste {
		t.Errorf("segunda vez: %v", err)
	}
	for _, malo := range []string{"x; rm -rf ~", "../../etc", "a b-1", "", "Canon_G2110-abc", "-a"} {
		if err := Cancelar(context.Background(), malo); err == nil {
			t.Errorf("%q debería rechazarse", malo)
		}
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "rm -rf") {
		t.Error("un ID inválido llegó a ejecutarse")
	}
	t.Setenv("FAKE_CANCEL", "denegado")
	err := Cancelar(context.Background(), "Canon_G2110-30")
	if a, ok := err.(*ErrAccion); !ok || !strings.HasPrefix(a.Accion, "sudo cancel") {
		t.Errorf("denegado: %v", err)
	}
}

func TestImprimirUnoRetieneLosDemasYHabilita(t *testing.T) {
	log, jobs := entornoCups(t, true, true,
		linea(25, "a.pdf", "pending", miUsuario(), 100),
		linea(26, "b.pdf", "pending", miUsuario(), 90),
		linea(27, "c.pdf", "pending-held", miUsuario(), 80))
	if errs := ImprimirUno(context.Background(), "Canon_G2110", "Canon_G2110-26"); len(errs) != 0 {
		t.Fatal(errs)
	}
	b, _ := os.ReadFile(jobs)
	s := string(b)
	if !strings.Contains(s, "25,\"a.pdf\",pending-held") || !strings.Contains(s, "26,\"b.pdf\",pending,") || !strings.Contains(s, "27,\"c.pdf\",pending-held") {
		t.Errorf("estados:\n%s", s)
	}
	l, _ := os.ReadFile(log)
	if !strings.Contains(string(l), "-i Canon_G2110-25 -H hold") || !strings.Contains(string(l), "cupsenable Canon_G2110") {
		t.Errorf("log:\n%s", l)
	}
	if strings.Index(string(l), "-H hold") > strings.Index(string(l), "cupsenable") {
		t.Error("hay que retener ANTES de habilitar la cola, o saldrían todos")
	}
	if errs := ImprimirUno(context.Background(), "Canon_G2110", "Canon_G2110-99"); len(errs) != 1 || errs[0] != errYaNoExiste {
		t.Errorf("trabajo inexistente: %v", errs)
	}
}

func TestImprimirTodosYCancelarTodos(t *testing.T) {
	log, jobs := entornoCups(t, true, true,
		linea(25, "a.pdf", "pending-held", miUsuario(), 100),
		linea(26, "b.pdf", "pending", miUsuario(), 90),
		linea(27, "ajeno.pdf", "pending", "otrouser", 80))
	if errs := ImprimirTodos(context.Background(), "Canon_G2110"); len(errs) != 0 {
		t.Fatal(errs)
	}
	b, _ := os.ReadFile(jobs)
	if strings.Contains(string(b), "pending-held") {
		t.Errorf("deberían estar liberados:\n%s", b)
	}
	if l, _ := os.ReadFile(log); !strings.Contains(string(l), "cupsenable") {
		t.Error("debía habilitar la cola")
	}
	if errs := CancelarTodos(context.Background(), "Canon_G2110"); len(errs) != 0 {
		t.Fatal(errs)
	}
	b, _ = os.ReadFile(jobs)
	if !strings.Contains(string(b), "ajeno.pdf") || strings.Contains(string(b), "a.pdf") {
		t.Errorf("solo debe cancelar los propios:\n%s", b)
	}
}

func TestSubcomandoJSON(t *testing.T) {
	entornoCups(t, true, false, linea(25, "a.pdf", "pending", miUsuario(), 100))
	var out, errb bytes.Buffer
	ok, code := subcomando(context.Background(), []string{"estado", "--json"}, configPrueba(), &out, &errb)
	var e Estado
	if !ok || code != 0 || json.Unmarshal(out.Bytes(), &e) != nil || e.Estado != "espera" {
		t.Fatalf("ok=%v code=%d out=%s err=%s", ok, code, out.String(), errb.String())
	}
	out.Reset()
	_, code = subcomando(context.Background(), []string{"cancelar", "Canon_G2110-25", "--json"}, configPrueba(), &out, &errb)
	var r Resultado
	if code != 0 || json.Unmarshal(out.Bytes(), &r) != nil || !r.OK || r.Estado == nil || len(r.Estado.Trabajos) != 0 {
		t.Fatalf("code=%d %s", code, out.String())
	}
	out.Reset()
	_, code = subcomando(context.Background(), []string{"cancelar", "--json"}, configPrueba(), &out, &errb)
	if code != 2 {
		t.Errorf("sin id debe ser error de uso, code=%d", code)
	}
	if ok, _ := subcomando(context.Background(), []string{"archivo.pdf"}, configPrueba(), &out, &errb); ok {
		t.Error("un archivo no es un subcomando")
	}
	// id al final o antes del flag
	entornoCups(t, true, false, linea(30, "b.pdf", "pending", miUsuario(), 5))
	out.Reset()
	if _, code = subcomando(context.Background(), []string{"retener", "--json", "Canon_G2110-30"}, configPrueba(), &out, &errb); code != 0 {
		t.Errorf("code=%d %s %s", code, out.String(), errb.String())
	}
}

func TestRegistroLimite(t *testing.T) {
	t.Setenv("PRINT_ESTADO", t.TempDir())
	for i := 0; i < 230; i++ {
		RegistrarEnvio(fmt.Sprintf("Canon_G2110-%d", i), "x.pdf")
	}
	if n := len(LeerRegistro()); n != 200 {
		t.Errorf("registro %d", n)
	}
}

func TestEdadHumana(t *testing.T) {
	casos := map[int64]string{-1: "—", 5: "ahora", 300: "hace 5 min", 7300: "hace 2 h", 100000: "ayer", 400000: "hace 4 días"}
	for s, w := range casos {
		if g := edadHumana(s); g != w {
			t.Errorf("%d: %q != %q", s, g, w)
		}
	}
}

func TestConsultaIPPDiceQuienPregunta(t *testing.T) {
	// Sin requesting-user-name, CUPS oculta nombre y dueño del trabajo: el
	// trabajo propio parecía «de otro usuario» y se bloqueaban sus acciones.
	if !strings.Contains(string(ippJobsTest), "requesting-user-name") {
		t.Fatal("la consulta de trabajos debe enviar requesting-user-name")
	}
}
