package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCandadoDelPerfilEntreProcesos(t *testing.T) {
	t.Setenv("PRINT_CACHE", t.TempDir())
	ctx := context.Background()
	soltarA, err := bloquearPerfil(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// otro intento mientras A lo tiene: no puede, y se rinde a tiempo con un error claro
	t0 := time.Now()
	if _, err := bloquearPerfil(ctx, 250*time.Millisecond); err == nil || time.Since(t0) < 200*time.Millisecond {
		t.Fatalf("debe esperar y fallar: %v tras %v", err, time.Since(t0))
	}
	// uno que espera más que A: lo consigue en cuanto A suelta
	var wg sync.WaitGroup
	wg.Add(1)
	var tomado time.Duration
	go func() {
		defer wg.Done()
		t1 := time.Now()
		soltar, err := bloquearPerfil(ctx, 3*time.Second)
		if err != nil {
			t.Error(err)
			return
		}
		tomado = time.Since(t1)
		soltar()
	}()
	time.Sleep(300 * time.Millisecond)
	soltarA()
	wg.Wait()
	if tomado < 250*time.Millisecond {
		t.Errorf("el segundo no debía pasar antes de que A soltara: %v", tomado)
	}
}

func TestConvertirConLibreOfficeFalso(t *testing.T) {
	log := conFalsos(t, "")
	t.Setenv("PRINT_CACHE", t.TempDir())
	pdf, _ := filepath.Abs("testdata/prueba.pdf")
	t.Setenv("FAKE_PDF", pdf)
	doc := filepath.Join(t.TempDir(), "informe.docx")
	os.WriteFile(doc, []byte("contenido"), 0o644)

	out, err := Convertir(context.Background(), doc, 5*time.Second)
	if err != nil || !strings.HasSuffix(out, "doc.pdf") {
		t.Fatalf("%q %v", out, err)
	}
	// segunda vez: sale de la caché y no vuelve a llamar a LibreOffice
	if _, err := Convertir(context.Background(), doc, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(log); strings.Count(string(b), "soffice") != 1 {
		t.Errorf("debía convertir una sola vez:\n%s", b)
	}
	// un PDF no se convierte
	if p, _ := Convertir(context.Background(), "x.pdf", time.Second); p != "x.pdf" {
		t.Errorf("un PDF se usa tal cual: %q", p)
	}
	// LibreOffice que no produce nada
	t.Setenv("FAKE_SOFFICE", "falla")
	doc2 := filepath.Join(t.TempDir(), "otro.docx")
	os.WriteFile(doc2, []byte("otro"), 0o644)
	if _, err := Convertir(context.Background(), doc2, 5*time.Second); err == nil || !strings.Contains(err.Error(), "no produjo") {
		t.Errorf("sin PDF debe fallar claro: %v", err)
	}
	// LibreOffice que se cuelga: se corta y avisa
	t.Setenv("FAKE_SOFFICE", "cuelga")
	doc3 := filepath.Join(t.TempDir(), "colgado.docx")
	os.WriteFile(doc3, []byte("colgado"), 0o644)
	t0 := time.Now()
	if _, err := Convertir(context.Background(), doc3, 400*time.Millisecond); err == nil || !strings.Contains(err.Error(), "tardó") || time.Since(t0) > 5*time.Second {
		t.Errorf("colgado: %v tras %v", err, time.Since(t0))
	}
}

func TestImpresoraSeDetectaPorElURIDeLaCola(t *testing.T) {
	conFalsos(t, "")
	d := t.TempDir()
	t.Setenv("PRINT_SYSUSB", d)
	dev := func(nombre, serie, fab, prod string) {
		p := filepath.Join(d, nombre)
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "idVendor"), []byte("1234\n"), 0o644) // otro ID distinto al fijo
		os.WriteFile(filepath.Join(p, "idProduct"), []byte("abcd\n"), 0o644)
		os.WriteFile(filepath.Join(p, "serial"), []byte(serie+"\n"), 0o644)
		os.WriteFile(filepath.Join(p, "manufacturer"), []byte(fab+"\n"), 0o644)
		os.WriteFile(filepath.Join(p, "product"), []byte(prod+"\n"), 0o644)
	}
	dev("1-1", "AAA111", "Canon", "G3010 series")
	ctx := context.Background()

	t.Setenv("FAKE_URI", "usb://Canon/G3010%20series?serial=AAA111&interface=1")
	if !impresoraConectada(ctx, "Canon_G2110") {
		t.Error("otra impresora USB (otro ID) debe detectarse por su número de serie")
	}
	// serie que el sistema no conoce (viene del ID IEEE-1284): se busca por fabricante y modelo
	t.Setenv("FAKE_URI", "usb://Canon/G3010%20series?serial=ZZZ999&interface=1")
	if !impresoraConectada(ctx, "Canon_G2110") {
		t.Error("con otra serie pero mismo fabricante y modelo se acepta (evita falsos «desconectada»)")
	}
	t.Setenv("FAKE_URI", "usb://Canon/G9999%20series?serial=ZZZ999&interface=1")
	if impresoraConectada(ctx, "Canon_G2110") {
		t.Error("otro modelo y otra serie: no es esta impresora")
	}
	t.Setenv("FAKE_URI", "usb://Canon/G3010%20series")
	if !impresoraConectada(ctx, "Canon_G2110") {
		t.Error("sin serie, por fabricante y modelo")
	}
	t.Setenv("FAKE_URI", "usb://Epson/L3210")
	if impresoraConectada(ctx, "Canon_G2110") {
		t.Error("otro fabricante")
	}
	t.Setenv("FAKE_URI", "ipp://192.168.1.50/ipp/print")
	if !impresoraConectada(ctx, "Canon_G2110") {
		t.Error("una impresora de red no se puede comprobar por USB: no debe marcarse como desconectada")
	}
	// sin device-uri legible no se puede comprobar: no se marca desconectada
	t.Setenv("FAKE_URI", "")
	if !impresoraConectada(ctx, "Canon_G2110") {
		t.Error("sin URI no se puede saber: no debe marcar desconectada")
	}
	if uriDispositivo(ctx, "x; rm -rf /") != "" {
		t.Error("nombre de cola inválido no debe consultarse")
	}
}

func TestPDFDanadoSeRechaza(t *testing.T) {
	if !existe("qpdf") || !existe("pdfinfo") {
		t.Skip("faltan qpdf o pdfinfo")
	}
	bueno := pdfNumerado(t, 3)
	if _, err := AnalizarPDF(context.Background(), bueno); err != nil {
		t.Fatalf("un PDF sano no debe fallar: %v", err)
	}
	datos, _ := os.ReadFile(bueno)
	roto := filepath.Join(t.TempDir(), "roto.pdf")
	os.WriteFile(roto, datos[:len(datos)/2], 0o644)
	if _, err := AnalizarPDF(context.Background(), roto); err != ErrPDFDanado {
		t.Errorf("un PDF cortado debe rechazarse: %v", err)
	}
}

func TestSubcomandoCarpetaSinTerminal(t *testing.T) {
	if c := noInteractivo(context.Background(), configPorDefecto(), argumentos{ruta: t.TempDir(), copias: 1}); c == 0 {
		t.Error("una carpeta no se puede imprimir")
	}
}

func TestMensajeTextoSinEspaciosSobrantes(t *testing.T) {
	if g := (Mensaje{"El archivo está vacío.", ""}).Texto(); g != "El archivo está vacío." {
		t.Errorf("%q", g)
	}
	if g := (Mensaje{"No existe", "Revisa la ruta"}).Texto(); g != "No existe. Revisa la ruta" {
		t.Errorf("%q", g)
	}
	if m := Validar("/tmp/x.jpg"); m == nil || strings.Contains(m.Que, "print imprime") || strings.HasSuffix(m.Texto(), " ") {
		t.Errorf("%+v", m)
	}
}

func TestConfiguracionDelArchivo(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("PATH", t.TempDir()) // sin lpstat: no hay cola predeterminada
	if c := CargarConfig(); c != configPorDefecto() {
		t.Errorf("sin archivo: valores por defecto, no %+v", c)
	}
	os.MkdirAll(filepath.Join(h, ".config", "print"), 0o755)
	os.WriteFile(filepath.Join(h, ".config", "print", "config.toml"), []byte(`
# comentario
impresora = "Otra_Cola"   # al final
tinta = "normal"
animacion = false
ascii = true
giro = "tortita"
timeout_soffice = 90
confirmar_voltea = true
clave_desconocida = 1
`), 0o644)
	c := CargarConfig()
	if c.Impresora != "Otra_Cola" || c.Tinta != "normal" || c.Animacion || !c.ASCII || c.Giro != "tortita" || c.TimeoutSoffice != 90 {
		t.Errorf("%+v", c)
	}
	os.WriteFile(filepath.Join(h, ".config", "print", "config.toml"), []byte("timeout_soffice = abc\n"), 0o644)
	if CargarConfig().TimeoutSoffice != 60 {
		t.Error("un valor inválido no debe pisar el defecto")
	}
}

func TestListarYBuscarDocumentos(t *testing.T) {
	raiz := t.TempDir()
	crear := func(rel string, antes time.Duration) {
		p := filepath.Join(raiz, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
		ts := time.Now().Add(-antes)
		os.Chtimes(p, ts, ts)
	}
	crear("informe.pdf", time.Hour)
	crear("Guía.docx", time.Minute)
	crear("sub/a.odt", 2*time.Hour)
	crear("foto.jpg", 0)               // imágenes: no se muestran
	crear(".oculta/secreto.pdf", 0)    // carpetas ocultas: no
	crear("node_modules/x/y.pdf", 0)   // node_modules: no
	crear("a/b/c/d/e/profundo.pdf", 0) // demasiado hondo
	l := ListarDocumentos([]string{raiz}, 3)
	var nombres []string
	for _, a := range l {
		nombres = append(nombres, a.Nombre)
	}
	if strings.Join(nombres, ",") != "Guía.docx,informe.pdf,a.odt" {
		t.Errorf("deben salir solo los documentos, el más reciente primero: %v", nombres)
	}
	if r := Buscar(l, "guia"); len(r) != 1 || r[0].Nombre != "Guía.docx" {
		t.Errorf("sin acentos ni mayúsculas: %+v", r)
	}
	if r := Buscar(l, "zzz"); len(r) != 0 {
		t.Errorf("sin coincidencias: %+v", r)
	}
	if r := Buscar(l, ""); len(r) != 3 {
		t.Errorf("búsqueda vacía muestra todo: %d", len(r))
	}
}

func TestCajaDeBusquedaMuestraLaAyudaCompleta(t *testing.T) {
	m := nuevoModelo(context.Background(), configPorDefecto(), "", false, false, true)
	m.w, m.h = 100, 40
	m.setPant(pBuscador)
	if v := m.View(); !strings.Contains(v, "escribe para buscar") {
		t.Errorf("la ayuda se recortaba a una letra («e»):\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if m.busq.Width < 10 || m.busq.Width > 60 {
		t.Errorf("el ancho del campo se ajusta a la ventana: %d", m.busq.Width)
	}
}

// ---- flujos completos de impresión (con CUPS falso)

func ejecutar1(m *model, c tea.Cmd) {
	if c == nil {
		return
	}
	if msg := c(); msg != nil {
		m.Update(msg)
	}
}

func TestFlujoUnaCaraDeProntoAListo(t *testing.T) {
	log := conFalsos(t, "")
	t.Setenv("PRINT_ESTADO", t.TempDir())
	m := modeloBase(t)
	m.pant, m.caras, m.copias = pVista, 0, 2
	m.pdf = "/x/informe.pdf"
	ejecutar1(m, m.empezarImpresion())
	if m.pant != pImprimiendo || m.job != "Canon_G2110-42" {
		t.Fatalf("tras enviar: pant=%v job=%q", m.pant, m.job)
	}
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "-n 2") || !strings.Contains(string(b), "-- /x/informe.pdf") {
		t.Errorf("lp: %s", b)
	}
	if r := LeerRegistro(); r["Canon_G2110-42"].Archivo != "informe.pdf" {
		t.Errorf("registro de envíos: %+v", r)
	}
	m.Update(msgPoll{job: "Canon_G2110-42", st: TrabajoTerminado, final: "completed"})
	if m.pant != pListo {
		t.Errorf("pant=%v", m.pant)
	}
	v := m.View()
	if !strings.Contains(v, "Hojas") || strings.Contains(v, "Canon_G2110-42") {
		t.Errorf("resumen final:\n%s", v)
	}
	// un trabajo que sigue activo no avanza
	m2 := modeloBase(t)
	m2.pant, m2.job, m2.jobs = pImprimiendo, "Canon_G2110-9", []string{"Canon_G2110-9"}
	m2.Update(msgPoll{job: "Canon_G2110-9", st: TrabajoActivo})
	if m2.pant != pImprimiendo {
		t.Error("activo: sigue imprimiendo")
	}
	// una respuesta de otro trabajo se ignora
	m2.Update(msgPoll{job: "Canon_G2110-1", st: TrabajoTerminado, final: "completed"})
	if m2.pant != pImprimiendo {
		t.Error("respuesta obsoleta: se ignora")
	}
}

func TestFlujoDobleCaraCompleto(t *testing.T) {
	log := conFalsos(t, "")
	t.Setenv("PRINT_ESTADO", t.TempDir())
	t.Setenv("PRINT_CACHE", t.TempDir())
	pdf := pdfNumerado(t, 4)
	m := modeloBase(t)
	m.ruta, m.pdf, m.caras, m.copias = pdf, pdf, 1, 1
	if err := m.prepararDuplex(); err != nil {
		t.Fatal(err)
	}
	m.pant = pVista
	ejecutar1(m, m.empezarImpresion())
	if m.pant != pImprimiendo || m.pasada != 1 {
		t.Fatalf("pasada 1: pant=%v pasada=%d", m.pant, m.pasada)
	}
	m.Update(msgPoll{job: m.job, st: TrabajoTerminado, final: "completed"})
	if m.pant != pVolteo || CargarSesionSinFiltro(t) == nil {
		t.Fatalf("debe pedir el volteo y guardar la sesión: pant=%v", m.pant)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ejecutar1(m, cmd)
	if m.pant != pImprimiendo || m.pasada != 2 {
		t.Fatalf("pasada 2: pant=%v pasada=%d", m.pant, m.pasada)
	}
	m.Update(msgPoll{job: m.job, st: TrabajoTerminado, final: "completed"})
	if m.pant != pListo {
		t.Fatalf("fin: pant=%v", m.pant)
	}
	if _, err := os.Stat(rutaSesion()); err == nil {
		t.Error("la sesión debe borrarse al terminar")
	}
	b, _ := os.ReadFile(log)
	lps := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lps) != 2 || !strings.Contains(lps[0], "pasada1_") || !strings.Contains(lps[1], "pasada2_") {
		t.Errorf("deben salir dos trabajos, primero los reversos y luego los frentes:\n%s", b)
	}
	if strings.Contains(string(b), "page-ranges") {
		t.Error("cada pasada imprime su PDF reordenado entero")
	}
	// lo que de verdad llega a la impresora: reversos 4,2 primero y luego frentes 3,1
	archivo := func(linea string) string { return linea[strings.LastIndex(linea, "-- ")+3:] }
	for i, quiero := range [][]string{{"P4", "P2"}, {"P3", "P1"}} {
		f := archivo(lps[i])
		for h, w := range quiero {
			if g := textoDePagina(t, f, h+1); g != w {
				t.Errorf("pasada %d, hoja %d: %q != %q", i+1, h+1, g, w)
			}
		}
	}
}

func TestTeclasDelBuscadorYDelMenu(t *testing.T) {
	m := modeloBase(t)
	m.pant = pBuscador
	m.archivos = []Archivo{{Ruta: "/x/a.pdf", Nombre: "a.pdf"}, {Ruta: "/x/b.pdf", Nombre: "b.pdf"}}
	m.coin = Buscar(m.archivos, "")
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cur != 1 {
		t.Errorf("↓ baja: %d", m.cur)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cur != 1 {
		t.Error("no pasa del último")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cur != 0 {
		t.Error("↑ sube")
	}
	// menú: esc vuelve al buscador, q sale
	m.pant = pMenu
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pant != pBuscador {
		t.Errorf("esc: %v", m.pant)
	}
	m.pant = pMenu
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil || !m.fin {
		t.Error("q sale")
	}
	// rango escrito a mano
	m2 := modeloBase(t)
	m2.fila, m2.modoRango = rPaginas, true
	for _, r := range "1-3,x" {
		m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m2.rangoTxt != "1-3," {
		t.Errorf("solo admite dígitos, coma y guion: %q", m2.rangoTxt)
	}
	m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m2.rangoTxt != "1-3" {
		t.Errorf("borrar: %q", m2.rangoTxt)
	}
}
