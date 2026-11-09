package main

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func conFalsos(t *testing.T, modo string) string {
	t.Helper()
	bin, _ := filepath.Abs("testdata/bin")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_MODO", modo)
	log := filepath.Join(t.TempDir(), "lp.log")
	t.Setenv("FAKE_LOG", log)
	return log
}

func TestEnviarArgsSeparados(t *testing.T) {
	log := conFalsos(t, "")
	args := ConstruirLP(Opciones{Cola: "Canon_G2110", Tinta: "color"}, "a b; $(x).pdf")
	id, err := Enviar(context.Background(), args)
	if err != nil || id != "Canon_G2110-42" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-- a b; $(x).pdf") {
		t.Errorf("log: %s", b)
	}
}

func TestEnviarError(t *testing.T) {
	conFalsos(t, "error")
	_, err := Enviar(context.Background(), []string{"-d", "x"})
	e, ok := err.(*ErrLP)
	if !ok {
		t.Fatalf("%T", err)
	}
	if m := TraducirErrorLP(e.Stderr); !strings.Contains(m.Que, "no existe") {
		t.Errorf("%+v", m)
	}
}

func TestColaYTrabajo(t *testing.T) {
	conFalsos(t, "")
	e, _ := LeerCola(context.Background(), "Canon_G2110")
	if !e.Existe || !e.Habilitada || !e.Aceptando {
		t.Errorf("%+v", e)
	}
	if ConsultarTrabajo(context.Background(), "Canon_G2110", "Canon_G2110-42") != TrabajoTerminado {
		t.Error("debería estar terminado")
	}
	t.Setenv("FAKE_MODO", "activo")
	if ConsultarTrabajo(context.Background(), "Canon_G2110", "Canon_G2110-42") != TrabajoActivo {
		t.Error("debería estar activo")
	}
	t.Setenv("FAKE_MODO", "parada")
	e, _ = LeerCola(context.Background(), "Canon_G2110")
	if e.Habilitada || e.Aceptando || !strings.Contains(e.Razon, "Unplugged") {
		t.Errorf("%+v", e)
	}
	e, _ = LeerCola(context.Background(), "NoExiste")
	if e.Existe {
		t.Error("no debería existir")
	}
}

func TestPreflightParada(t *testing.T) {
	conFalsos(t, "parada")
	t.Setenv("PRINT_SYSUSB", t.TempDir())
	t.Setenv("FAKE_URI", "usb://Ejemplo/Modelo%20series?serial=NOEXISTE&interface=1")
	_, hs := Preflight(context.Background(), "Canon_G2110")
	ids := map[string]bool{}
	for _, h := range hs {
		ids[h.ID] = true
	}
	for _, id := range []string{"P-4", "P-5", "P-6"} {
		if !ids[id] {
			t.Errorf("falta %s en %v", id, hs)
		}
	}
}

func TestPreflightSano(t *testing.T) {
	conFalsos(t, "")
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "1-1"), 0o755)
	os.WriteFile(filepath.Join(d, "1-1", "idVendor"), []byte("1234\n"), 0o644)
	os.WriteFile(filepath.Join(d, "1-1", "idProduct"), []byte("5678\n"), 0o644)
	os.WriteFile(filepath.Join(d, "1-1", "manufacturer"), []byte("Ejemplo\n"), 0o644)
	os.WriteFile(filepath.Join(d, "1-1", "product"), []byte("Modelo series\n"), 0o644)
	t.Setenv("PRINT_SYSUSB", d)
	t.Setenv("FAKE_URI", "usb://Ejemplo/Modelo%20series?serial=ABC&interface=1")
	_, hs := Preflight(context.Background(), "Canon_G2110")
	for _, h := range hs {
		if h.Bloqueante || h.Grave {
			t.Errorf("no debería haber problemas graves: %+v", h)
		}
	}
}

func TestAnalizarPDF(t *testing.T) {
	if !existe("pdfinfo") {
		t.Skip("falta pdfinfo")
	}
	info, err := AnalizarPDF(context.Background(), "testdata/prueba.pdf")
	if err != nil || info.Paginas != 1 || info.Papel != "Letter" {
		t.Errorf("%+v %v", info, err)
	}
	if NombrePapel(842, 595) != "A4" {
		t.Error("A4 apaisado")
	}
}

func TestSesion(t *testing.T) {
	t.Setenv("PRINT_ESTADO", t.TempDir())
	f := filepath.Join(t.TempDir(), "x.pdf")
	os.WriteFile(f, []byte("%PDF"), 0o644)
	st, _ := os.Stat(f)
	s := &Sesion{Archivo: f, Mtime: st.ModTime().Unix(), Estado: "ESPERA_VOLTEO", A: []int{1, 3}}
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}
	if g := CargarSesion(); g == nil || g.Estado != "ESPERA_VOLTEO" {
		t.Errorf("%+v", g)
	}
	os.WriteFile(f, []byte("%PDF cambiado"), 0o644)
	os.Chtimes(f, st.ModTime().Add(5e9), st.ModTime().Add(5e9))
	if CargarSesion() != nil {
		t.Error("archivo cambiado: debe descartarse")
	}
}

func TestBuscarYValidar(t *testing.T) {
	l := []Archivo{{Nombre: "Guía_Estudio_Software_I.docx"}, {Nombre: "informe.pdf"}, {Nombre: "Informe_Laboratorio_4.odt"}}
	r := Buscar(l, "guia sw")
	if len(r) != 1 || r[0].Nombre != "Guía_Estudio_Software_I.docx" {
		t.Errorf("%+v", r)
	}
	if r := Buscar(l, "inf"); len(r) != 2 {
		t.Errorf("%d", len(r))
	}
	if m := Validar("/tmp/x.jpg"); m == nil {
		t.Error("jpg")
	}
	if EsDocumento("a.jpg") || !EsDocumento("A.DOCX") {
		t.Error("extensiones")
	}
}

func TestEjecutarTimeout(t *testing.T) {
	_, _, err := ejecutar(context.Background(), 200e6, "sleep", "5")
	if err != ErrTiempo {
		t.Errorf("%v", err)
	}
}

func TestPrepararReal(t *testing.T) {
	if !existe("gs") {
		t.Skip("sin gs")
	}
	t.Setenv("PRINT_CACHE", t.TempDir())
	// prueba.pdf es Letter; lo dejamos en A4 a 1:1: cabe, queda arriba y centrado
	out, err := PrepararReal(context.Background(), "testdata/prueba.pdf", 612, 792, "A4")
	if err != nil {
		t.Fatal(err)
	}
	info, _ := AnalizarPDF(context.Background(), out)
	if info.Papel != "A4" {
		t.Errorf("papel %q", info.Papel)
	}
	if _, err := PrepararReal(context.Background(), "testdata/prueba.pdf", 612, 792, "Rarisimo"); err == nil {
		t.Error("papel desconocido debería fallar")
	}
}

func TestDesajuste(t *testing.T) {
	m := &model{papel: "Letter", info: InfoPDF{Papel: "A4", Ancho: 595, Alto: 842}}
	if !m.hayDesajuste() || m.escalaArg() != "fit" || !m.visible(rEscala) {
		t.Error("A4 en Carta: debería haber desajuste y fit")
	}
	m.iEscala = 3
	if m.escalaArg() != "fill" {
		t.Error("cubrir = fill")
	}
	m.iEscala = 2
	if m.escalaArg() != "" {
		t.Error("estirar prepara su propio PDF y no usa print-scaling")
	}
	m.iEscala = 1
	if m.escalaArg() != "" {
		t.Error("real no usa print-scaling")
	}
	m.info.Papel = "Letter"
	if m.hayDesajuste() || m.visible(rEscala) || m.escalaArg() != "" {
		t.Error("sin desajuste no hay fila ni escala")
	}
}

func TestExtrasNoPisanElMenu(t *testing.T) {
	conFalsos(t, "")
	p := clavesPermitidas()
	for _, k := range []string{"StpOrientation", "ColorModel", "PageSize", "MediaType", "StpFullBleed"} {
		if p[k] {
			t.Errorf("%s no debería poder editarse", k)
		}
	}
	if !p["StpBrightness"] {
		t.Error("StpBrightness debería estar permitida")
	}
	// lpoptions falso: la salida de un PPD real no está disponible en CI.
	t.Setenv("FAKE_LPOPTIONS", "StpiShrinkOutput/Shrink page if necessary to fit borders: Shrink Crop Expand")
	if vs := ValoresPPD("Canon_G2110", "StpiShrinkOutput"); indiceDe(vs, "Shrink") < 0 {
		t.Errorf("valores PPD: %v", vs)
	}
}

func TestApaisadoCuentaComoDesajuste(t *testing.T) {
	m := &model{papel: "A4", info: InfoPDF{Papel: "A4", Ancho: 842, Alto: 595}}
	if !m.apaisado() || !m.hayDesajuste() {
		t.Fatal("un A4 apaisado debe contar como desajuste")
	}
	if m.escalaArg() != "fit" {
		t.Error("por defecto Ajustar (fit)")
	}
	m.iEscala = 1 // tamaño real: se prepara su propio PDF, sin opciones de CUPS
	if m.escalaArg() != "" {
		t.Error("tamaño real no usa print-scaling")
	}
}

func TestLimpiarCacheProtegeSesion(t *testing.T) {
	t.Setenv("PRINT_CACHE", t.TempDir())
	viejo := time.Now().Add(-48 * time.Hour)
	for _, n := range []string{"protegido", "basura"} {
		d := filepath.Join(dirCache(), n)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "doc.pdf"), []byte("x"), 0o644)
		os.Chtimes(d, viejo, viejo)
	}
	LimpiarCache(filepath.Join(dirCache(), "protegido", "doc.pdf"))
	if _, err := os.Stat(filepath.Join(dirCache(), "protegido")); err != nil {
		t.Error("el PDF de la sesión no debe borrarse")
	}
	if _, err := os.Stat(filepath.Join(dirCache(), "basura")); err == nil {
		t.Error("lo viejo sin sesión sí debe borrarse")
	}
}

func TestSesionGuardaBordesYExtra(t *testing.T) {
	m := &model{papel: "Letter", bordes: true, extra: []string{"StpBrightness=500"}, info: InfoPDF{Paginas: 4},
		modoRango: false, copias: 1}
	s := m.sesion("ESPERA_VOLTEO")
	if s.Opciones["bordes"] != "1" || s.Opciones["extra"] != "StpBrightness=500" {
		t.Errorf("%v", s.Opciones)
	}
}

func TestErrorEnPasada2NoRecomienza(t *testing.T) {
	m := &model{ctx: context.Background(), pant: pImprimiendo, caras: 1, pasada: 2, jobs: []string{"Canon_G2110-41"}}
	m.Update(msgEnviado{err: &ErrLP{Err: os.ErrInvalid, Stderr: "lp: Error - The printer or class does not exist."}})
	if m.pant != pError || m.volver != pVolteo || m.retryPasada != 2 {
		t.Errorf("pant=%v volver=%v retry=%d", m.pant, m.volver, m.retryPasada)
	}
	// error al enviar el primer trabajo de todos: vuelve a la vista previa
	m2 := &model{ctx: context.Background(), pant: pImprimiendo, caras: 1, pasada: 1}
	m2.Update(msgEnviado{err: os.ErrInvalid})
	if m2.volver != pVista || m2.retryPasada != 0 {
		t.Errorf("volver=%v retry=%d", m2.volver, m2.retryPasada)
	}
}

func TestNoInteractivoRespetaDryRun(t *testing.T) {
	log := conFalsos(t, "")
	t.Setenv("PRINT_CACHE", t.TempDir())
	a := argumentos{ruta: "testdata/prueba.pdf", dry: true, copias: 2, paginas: "1"}
	if noInteractivo(context.Background(), configPorDefecto(), a) != 0 {
		t.Fatal("falló")
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Errorf("--dry-run llamó a lp: %s", b)
	}
	a.dry = false
	if noInteractivo(context.Background(), configPorDefecto(), a) != 0 {
		t.Fatal("falló")
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-n 2") || !strings.Contains(string(b), "page-ranges=1") {
		t.Errorf("opciones por flag: %s", b)
	}
	if noInteractivo(context.Background(), configPorDefecto(), argumentos{ruta: "testdata/prueba.pdf", copias: 1, papel: "A5"}) == 0 {
		t.Error("A5 no existe en el PPD")
	}
}

// Fabrica un PDF de n páginas con el número de página como texto.
func pdfNumerado(t *testing.T, n int) string {
	t.Helper()
	if !existe("gs") || !existe("qpdf") || !existe("pdftotext") {
		t.Skip("faltan gs, qpdf o pdftotext")
	}
	d := t.TempDir()
	ps := "%!PS\n/Helvetica findfont 40 scalefont setfont\n"
	for i := 1; i <= n; i++ {
		ps += fmt.Sprintf("50 400 moveto (P%d) show showpage\n", i)
	}
	os.WriteFile(filepath.Join(d, "n.ps"), []byte(ps), 0o644)
	out := filepath.Join(d, "n.pdf")
	if o, err := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite", "-sOutputFile="+out, filepath.Join(d, "n.ps")).CombinedOutput(); err != nil {
		t.Fatalf("gs: %v %s", err, o)
	}
	return out
}

func textoDePagina(t *testing.T, pdf string, p int) string {
	t.Helper()
	b, _ := exec.Command("pdftotext", "-f", fmt.Sprint(p), "-l", fmt.Sprint(p), pdf, "-").Output()
	return strings.TrimSpace(string(b))
}

func TestPrepararPasadasReordenaElPDF(t *testing.T) {
	pdf := pdfNumerado(t, 4)
	t.Setenv("PRINT_CACHE", t.TempDir())
	p1, p2, blanco, err := PrepararPasadas(context.Background(), pdf, PlanificarDuplex([]int{1, 2, 3, 4}))
	if err != nil || blanco {
		t.Fatalf("con páginas pares no se añade hoja en blanco: %v %v", blanco, err)
	}
	// pasada 1: reversos de mayor a menor; pasada 2: frentes de mayor a menor
	for i, want := range []string{"P4", "P2"} {
		if g := textoDePagina(t, p1, i+1); g != want {
			t.Errorf("pasada 1, hoja %d: %q != %q", i+1, g, want)
		}
	}
	for i, want := range []string{"P3", "P1"} {
		if g := textoDePagina(t, p2, i+1); g != want {
			t.Errorf("pasada 2, hoja %d: %q != %q", i+1, g, want)
		}
	}
	// segunda vez: usa la caché y da lo mismo
	q1, _, _, err := PrepararPasadas(context.Background(), pdf, PlanificarDuplex([]int{1, 2, 3, 4}))
	if err != nil || q1 != p1 {
		t.Errorf("caché: %q %q %v", q1, p1, err)
	}
	// impar: 5 páginas -> print añade una hoja en blanco al principio de la pasada 1
	pdf5 := pdfNumerado(t, 5)
	q1b, p2b, blanco, err := PrepararPasadas(context.Background(), pdf5, PlanificarDuplex([]int{1, 2, 3, 4, 5}))
	if err != nil || !blanco {
		t.Fatalf("impar: debe añadir la hoja en blanco solo: %v %v", blanco, err)
	}
	if n := paginasDe(t, q1b); n != 3 {
		t.Errorf("pasada 1 impar debe tener 3 hojas (blanca, 4, 2), tiene %d", n)
	}
	for i, want := range []string{"", "P4", "P2"} {
		if g := textoDePagina(t, q1b, i+1); g != want {
			t.Errorf("impar pasada 1, hoja %d: %q != %q", i+1, g, want)
		}
	}
	for i, want := range []string{"P5", "P3", "P1"} {
		if g := textoDePagina(t, p2b, i+1); g != want {
			t.Errorf("impar pasada 2, hoja %d: %q != %q", i+1, g, want)
		}
	}
}

func paginasDe(t *testing.T, pdf string) int {
	t.Helper()
	b, _ := exec.Command("qpdf", "--show-npages", pdf).Output()
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func TestPrepararPasadasSinQpdf(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, _, _, err := PrepararPasadas(context.Background(), "/x.pdf", PlanificarDuplex([]int{1, 2})); err == nil {
		t.Error("sin qpdf debe fallar con un mensaje claro, no imprimir en otro orden")
	}
}

// bordes: ¿hay tinta pegada a cada lado de la imagen? (izq, der, arriba, abajo)
func bordesPintados(t *testing.T, pdf string) [4]bool {
	t.Helper()
	d := t.TempDir()
	if err := exec.Command("pdftoppm", "-r", "30", "-png", pdf, filepath.Join(d, "p")).Run(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(d, "p-1.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	oscuro := func(x, y int) bool {
		r, g, bl, _ := img.At(x, y).RGBA()
		return r+g+bl < 3*0x4000
	}
	cy, cx := (b.Min.Y+b.Max.Y)/2, (b.Min.X+b.Max.X)/2
	return [4]bool{oscuro(b.Min.X+1, cy), oscuro(b.Max.X-2, cy), oscuro(cx, b.Min.Y+1), oscuro(cx, b.Max.Y-2)}
}

func pdfConMarco(t *testing.T, ancho, alto int) string {
	t.Helper()
	if !existe("gs") || !existe("pdftoppm") {
		t.Skip("faltan gs o pdftoppm")
	}
	d := t.TempDir()
	ps := fmt.Sprintf("%%!PS\n<< /PageSize [%d %d] >> setpagedevice\n4 setlinewidth 2 2 %d %d rectstroke\n/Helvetica-Bold findfont 40 scalefont setfont 100 100 moveto (ARRIBA 1) show showpage\n", ancho, alto, ancho-4, alto-4)
	os.WriteFile(filepath.Join(d, "m.ps"), []byte(ps), 0o644)
	out := filepath.Join(d, "m.pdf")
	if o, err := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite", "-sOutputFile="+out, filepath.Join(d, "m.ps")).CombinedOutput(); err != nil {
		t.Fatalf("gs: %v %s", err, o)
	}
	return out
}

func TestEstirarLlenaTodaLaHojaSinCortar(t *testing.T) {
	pdf := pdfConMarco(t, 595, 842) // A4 con un marco pegado al borde
	t.Setenv("PRINT_CACHE", t.TempDir())
	out, err := PrepararEstirar(context.Background(), pdf, 595, 842, "Letter")
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := AnalizarPDF(context.Background(), out); info.Papel != "Letter" {
		t.Errorf("debe quedar en Carta: %+v", info)
	}
	if b := bordesPintados(t, out); b != [4]bool{true, true, true, true} {
		t.Errorf("el marco debe tocar los cuatro bordes (nada cortado ni franjas): %v", b)
	}
}

func TestEstirarApaisadoSeGiraYLlena(t *testing.T) {
	pdf := pdfConMarco(t, 842, 595)
	t.Setenv("PRINT_CACHE", t.TempDir())
	out, err := PrepararEstirar(context.Background(), pdf, 842, 595, "Letter")
	if err != nil {
		t.Fatal(err)
	}
	info, _ := AnalizarPDF(context.Background(), out)
	if info.Papel != "Letter" || info.Ancho > info.Alto {
		t.Errorf("debe quedar vertical en Carta: %+v", info)
	}
	if b := bordesPintados(t, out); b != [4]bool{true, true, true, true} {
		t.Errorf("apaisado estirado: %v", b)
	}
	if _, err := PrepararEstirar(context.Background(), pdf, 842, 595, "Rarisimo"); err == nil {
		t.Error("papel desconocido debe fallar")
	}
	if _, err := PrepararEstirar(context.Background(), pdf, 0, 0, "Letter"); err == nil {
		t.Error("sin tamaño de origen debe fallar")
	}
}

// Marco con un cuadrado rojo en la esquina inferior izquierda y uno azul en la
// superior derecha, para ver hacia dónde gira y qué parte se corta.
func pdfApaisadoConMarcas(t *testing.T) string {
	t.Helper()
	if !existe("gs") || !existe("pdftoppm") {
		t.Skip("faltan gs o pdftoppm")
	}
	d := t.TempDir()
	ps := "%!PS\n<< /PageSize [842 595] >> setpagedevice\n6 setlinewidth 3 3 836 589 rectstroke\n" +
		"1 0 0 setrgbcolor 3 3 80 80 rectfill\n0 0 1 setrgbcolor 759 512 80 80 rectfill\n" +
		"0 setgray /Helvetica-Bold findfont 40 scalefont setfont 300 280 moveto (ARRIBA 1) show showpage\n"
	os.WriteFile(filepath.Join(d, "l.ps"), []byte(ps), 0o644)
	out := filepath.Join(d, "l.pdf")
	if o, err := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite", "-sOutputFile="+out, filepath.Join(d, "l.ps")).CombinedOutput(); err != nil {
		t.Fatalf("gs: %v %s", err, o)
	}
	return out
}

func pixelEn(t *testing.T, pdf string, x, y int) (int, int, int, int, int) {
	t.Helper()
	d := t.TempDir()
	if err := exec.Command("pdftoppm", "-r", "30", "-png", pdf, filepath.Join(d, "p")).Run(); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(filepath.Join(d, "p-1.png"))
	defer f.Close()
	img, _ := png.Decode(f)
	b := img.Bounds()
	if y < 0 {
		y = b.Max.Y + y
	}
	if x < 0 {
		x = b.Max.X + x
	}
	r, g, bl, _ := img.At(x, y).RGBA()
	return int(r >> 8), int(g >> 8), int(bl >> 8), b.Max.X, b.Max.Y
}

func TestTamanoRealApaisadoSeGiraEnSentidoHorarioYCortaPorAbajo(t *testing.T) {
	pdf := pdfApaisadoConMarcas(t)
	t.Setenv("PRINT_CACHE", t.TempDir())
	out, err := PrepararReal(context.Background(), pdf, 842, 595, "Letter")
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := AnalizarPDF(context.Background(), out); info.Papel != "Letter" || info.Ancho > info.Alto {
		t.Fatalf("debe quedar vertical en Carta: %+v", info)
	}
	if b, _ := exec.Command("pdfinfo", out).Output(); !strings.Contains(string(b), "Page rot:        0") {
		t.Errorf("la página no debe llevar /Rotate (Ghostscript lo añade según el texto y deshace el giro):\n%s", b)
	}
	// la esquina roja (abajo-izquierda del original) debe quedar ARRIBA a la izquierda,
	// que es hacia donde CUPS gira al ajustar (sentido horario)
	if r, g, b, _, _ := pixelEn(t, out, 20, 20); r < 200 || g > 60 || b > 60 {
		t.Errorf("la esquina roja debe quedar arriba a la izquierda: %d,%d,%d", r, g, b)
	}
	// a tamaño real NO se reduce: el marco no toca los lados de la hoja (sobran ~8,5 pt)
	if r, g, b, _, _ := pixelEn(t, out, 1, 110); r+g+b < 700 {
		t.Errorf("el marco no debe llegar al borde izquierdo (no se escaló): %d,%d,%d", r, g, b)
	}
	// y se corta por abajo: del cuadrado azul (arriba-derecha del original) solo asoma
	// una tira en el borde inferior; el resto (unos 47 pt) queda fuera de la hoja
	if r, g, b, _, _ := pixelEn(t, out, -12, -3); b < 200 || r > 80 || g > 80 {
		t.Errorf("debe asomar la tira azul abajo a la derecha: %d,%d,%d", r, g, b)
	}
	if r, g, b, _, _ := pixelEn(t, out, -12, -25); r+g+b < 600 {
		t.Errorf("más arriba ya no hay azul (se cortó): %d,%d,%d", r, g, b)
	}
}

func TestEstirarApaisadoGiraEnElMismoSentido(t *testing.T) {
	pdf := pdfApaisadoConMarcas(t)
	t.Setenv("PRINT_CACHE", t.TempDir())
	out, err := PrepararEstirar(context.Background(), pdf, 842, 595, "Letter")
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _, _ := pixelEn(t, out, 14, 14); r < 200 || g > 60 || b > 60 {
		t.Errorf("rojo arriba a la izquierda: %d,%d,%d", r, g, b)
	}
	if r, g, b, _, _ := pixelEn(t, out, -14, -14); b < 200 || r > 60 || g > 60 {
		t.Errorf("azul abajo a la derecha: %d,%d,%d", r, g, b)
	}
}

func TestUnArchivoViejoDeLaCacheNoSeReutiliza(t *testing.T) {
	pdf := pdfApaisadoConMarcas(t)
	t.Setenv("PRINT_CACHE", t.TempDir())
	clave, _ := claveCache(pdf)
	dir := filepath.Join(dirCache(), clave)
	os.MkdirAll(dir, 0o755)
	// un resultado de una versión anterior (sin girar) con el nombre que se usaba entonces
	os.WriteFile(filepath.Join(dir, "real_Letter.pdf"), []byte("%PDF-viejo"), 0o644)
	os.WriteFile(filepath.Join(dir, "estirar_Letter.pdf"), []byte("%PDF-viejo"), 0o644)
	out, err := PrepararReal(context.Background(), pdf, 842, 595, "Letter")
	if err != nil || strings.HasSuffix(out, "real_Letter.pdf") {
		t.Fatalf("no debe reutilizar el archivo viejo: %q %v", out, err)
	}
	if r, g, b, _, _ := pixelEn(t, out, 20, 20); r < 200 || g > 60 || b > 60 {
		t.Errorf("el archivo nuevo sí debe estar girado: %d,%d,%d", r, g, b)
	}
	out, err = PrepararEstirar(context.Background(), pdf, 842, 595, "Letter")
	if err != nil || strings.HasSuffix(out, "estirar_Letter.pdf") {
		t.Fatalf("estirar tampoco: %q %v", out, err)
	}
}

func TestNombreImpresora(t *testing.T) {
	conFalsos(t, "")
	ctx := context.Background()

	t.Setenv("FAKE_LPOPTIONS", "printer-info='Impresora de prueba' printer-make-and-model='Modelo X'")
	if n := nombreImpresora(ctx, "Canon_G2110"); n != "Impresora de prueba" {
		t.Errorf("printer-info: %q", n)
	}

	t.Setenv("FAKE_LPOPTIONS", "printer-info='' printer-make-and-model='Modelo X'")
	if n := nombreImpresora(ctx, "Canon_G2110"); n != "Modelo X" {
		t.Errorf("printer-make-and-model: %q", n)
	}

	t.Setenv("FAKE_LPOPTIONS", "")
	if n := nombreImpresora(ctx, "Mi_Cola"); n != "Mi Cola" {
		t.Errorf("respaldo por cola: %q", n)
	}

	if n := nombreImpresora(ctx, ""); n != "Impresora" {
		t.Errorf("sin cola: %q", n)
	}
}

func TestLeerImpresoras(t *testing.T) {
	entornoCups(t, false, false)
	ls := LeerImpresoras(context.Background(), "Canon_G2110")
	if len(ls) != 1 {
		t.Fatalf("esperaba 1 cola: %+v", ls)
	}
	if got := ls[0]; got.Cola != "Canon_G2110" || got.Conectada ||
		got.Estado != "desconectada" || !got.Predeterminada {
		t.Errorf("desconectada: %+v", got)
	}

	entornoCups(t, true, false)
	ls = LeerImpresoras(context.Background(), "Canon_G2110")
	if len(ls) != 1 || !ls[0].Conectada || ls[0].Estado != "lista" {
		t.Errorf("conectada: %+v", ls)
	}
}
