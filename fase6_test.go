package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func modeloBase(t *testing.T) *model {
	t.Helper()
	t.Setenv("PRINT_ESTADO", t.TempDir())
	return &model{ctx: context.Background(), cfg: configPorDefecto(), sim: uni, w: 100, h: 40,
		cola: "Canon_G2110", nombre: "Impresora de prueba", pant: pMenu, copias: 1, copia: 1, pasada: 1, papel: "Letter",
		papelVis: []string{"Letter", "A4"}, todosPapeles: []string{"Letter", "A4"},
		info: InfoPDF{Paginas: 4}, ruta: "/x/informe.pdf", pdf: "/x/informe.pdf"}
}

func TestEstadoFinalDeUnTrabajo(t *testing.T) {
	entornoCups(t, true, false)
	d := filepath.Join(t.TempDir(), "done.csv")
	os.WriteFile(d, []byte("42,\"a.pdf\",canceled,none,usuario,26,1\n43,\"b.pdf\",completed,none,usuario,26,1\n44,\"c.pdf\",aborted,none,usuario,26,1\n"), 0o644)
	t.Setenv("FAKE_DONE", d)
	for id, want := range map[string]string{"Canon_G2110-42": "canceled", "Canon_G2110-43": "completed", "Canon_G2110-44": "aborted", "Canon_G2110-99": ""} {
		if g := EstadoFinal(context.Background(), "Canon_G2110", id); g != want {
			t.Errorf("%s: %q != %q", id, g, want)
		}
	}
	if EstadoFinal(context.Background(), "Canon_G2110", "x; rm -rf /") != "" {
		t.Error("ID inválido")
	}
}

func TestTrabajoCanceladoNoAvanzaAlVolteo(t *testing.T) {
	m := modeloBase(t)
	m.caras, m.pant, m.job, m.pasada = 1, pImprimiendo, "Canon_G2110-42", 1
	m.jobs = []string{"Canon_G2110-42"}
	m.Update(msgPoll{job: "Canon_G2110-42", st: TrabajoTerminado, final: "canceled"})
	if m.pant != pError || m.retryPasada != 1 {
		t.Errorf("un trabajo cancelado no debe llevar al volteo: pant=%v retry=%d", m.pant, m.retryPasada)
	}
	m2 := modeloBase(t)
	m2.caras, m2.pant, m2.job, m2.pasada = 1, pImprimiendo, "Canon_G2110-42", 1
	m2.Update(msgPoll{job: "Canon_G2110-42", st: TrabajoTerminado, final: "aborted"})
	if m2.pant != pError || !strings.Contains(m2.err.Que, "falló") {
		t.Errorf("abortado: %v %+v", m2.pant, m2.err)
	}
	m3 := modeloBase(t)
	m3.caras, m3.pant, m3.job, m3.pasada = 1, pImprimiendo, "Canon_G2110-42", 1
	m3.rangoTxt, m3.modoRango = "", false
	m3.Update(msgPoll{job: "Canon_G2110-42", st: TrabajoTerminado, final: "completed"})
	if m3.pant != pVolteo {
		t.Errorf("completado sí avanza: %v", m3.pant)
	}
}

func TestCancelarConDosPulsaciones(t *testing.T) {
	m := modeloBase(t)
	m.trabajosCola = []Trabajo{{ID: "Canon_G2110-25", Propio: true, Estado: "pendiente"}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil || !m.confirmCancel {
		t.Fatal("la primera pulsación solo pide confirmar")
	}
	if b := strings.Join(m.banda(), "\n"); !strings.Contains(b, "Pulsa x otra vez") {
		t.Errorf("banda: %q", b)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil || m.confirmCancel {
		t.Fatal("la segunda pulsación cancela")
	}
	// otra tecla en medio anula la confirmación
	m.confirmCancel = false
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.confirmCancel {
		t.Error("otra tecla debe anular la confirmación")
	}
	// sin trabajos propios no hace nada
	m.trabajosCola = []Trabajo{{ID: "a-1", Propio: false}}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil || m.confirmCancel {
		t.Error("los trabajos ajenos no se pueden cancelar")
	}
}

func TestHabilitarSoloSiHayUSB(t *testing.T) {
	m := modeloBase(t)
	m.hallazgos = []Hallazgo{{ID: "P-4", Grave: true, Mensaje: Mensaje{Que: "detenida"}}}
	if !m.colaDetenida() {
		t.Fatal("cola detenida con USB debe poder habilitarse")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Error("e debe lanzar la habilitación")
	}
	m.hallazgos = append(m.hallazgos, Hallazgo{ID: "P-6", Grave: true})
	if m.colaDetenida() {
		t.Error("sin USB no se ofrece habilitar")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd != nil {
		t.Error("sin USB e no hace nada")
	}
}

func TestAutoRefrescoYMsgAuto(t *testing.T) {
	m := modeloBase(t)
	m.hallazgos = []Hallazgo{{ID: "P-6", Grave: true}}
	m.autoUlt = time.Now().Add(-10 * time.Second)
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil || !m.autoBusy {
		t.Fatal("con un aviso de impresora debe refrescar solo")
	}
	m.Update(msgAuto{cola: "Canon_G2110", hs: nil, trabajos: []Trabajo{{ID: "a-1"}}})
	if m.autoBusy || m.pant != pMenu || len(m.trabajosCola) != 1 || len(m.hallazgos) != 0 {
		t.Errorf("msgAuto no debe cambiar de pantalla: busy=%v pant=%v", m.autoBusy, m.pant)
	}
	// sin avisos ni trabajos no hay refresco
	m2 := modeloBase(t)
	m2.autoUlt = time.Now().Add(-10 * time.Second)
	m2.Update(tickMsg(time.Now()))
	if m2.autoBusy {
		t.Error("sin nada que vigilar no debe consultar")
	}
}

func TestGuardaDeDobleEnvio(t *testing.T) {
	m := modeloBase(t)
	m.pant = pVista
	m.trabajosCola = []Trabajo{{ID: "Canon_G2110-25", Nombre: "informe.pdf", Origen: "print", Estado: "pendiente", EdadS: 120}}
	if m.avisoDuplicado() == "" {
		t.Fatal("debe avisar del duplicado")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !m.dupOK || m.dupAviso == "" {
		t.Fatalf("la primera vez solo avisa: cmd=%v dupOK=%v", cmd != nil, m.dupOK)
	}
	m.trabajosCola = []Trabajo{{Nombre: "otro.pdf", Origen: "print", Estado: "pendiente"}}
	if m.avisoDuplicado() != "" {
		t.Error("otro archivo no es duplicado")
	}
}

func TestMsgCanceladoVuelveAlMenu(t *testing.T) {
	m := modeloBase(t)
	m.pant = pImprimiendo
	m.Update(msgCancelado{pasada: true})
	if m.pant != pMenu || !strings.Contains(m.nota, "cancelados") {
		t.Errorf("pant=%v nota=%q", m.pant, m.nota)
	}
	m.pant = pMenu
	m.Update(msgCancelado{errs: []error{&ErrAccion{Mensaje{"sin permiso", "sudo cancel x-1"}}}})
	if !strings.Contains(m.nota, "sudo cancel") {
		t.Errorf("nota %q", m.nota)
	}
}

func TestVolteoUnEnterYListo(t *testing.T) {
	m := modeloBase(t)
	m.pant, m.caras = pVolteo, 1
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.enviandoP2 {
		t.Fatal("un Enter debe enviar la pasada 2, sin casilla ni cuenta atrás")
	}
	if _, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd2 != nil {
		t.Error("un segundo Enter no debe enviar otra vez")
	}
	v := m.View()
	for _, quitado := range []string{"Comprobado", "casilla", "Enviando la pasada 2 en"} {
		if strings.Contains(v, quitado) {
			t.Errorf("la pantalla ya no debe mostrar %q", quitado)
		}
	}
	m.Update(msgEnviado{id: "Canon_G2110-50"})
	if m.enviandoP2 {
		t.Error("tras enviar se libera el bloqueo")
	}
}

func TestOrientacionDelReversoSegunElGiro(t *testing.T) {
	m := modeloBase(t)
	m.caras, m.copias = 1, 1
	m.cfg.Giro = "libro" // por un borde largo: las dos caras van igual
	if m.reversoRotado() {
		t.Fatal("libro + vertical: no se gira")
	}
	for p := 1; p <= 2; p++ {
		if a := strings.Join(m.argumentos(p), " "); !strings.Contains(a, "StpOrientation=Portrait") || strings.Contains(a, "UpsideDown") {
			t.Errorf("pasada %d con giro de libro: %s", p, a)
		}
	}
	m.cfg.Giro = "tortita" // por un borde corto: los reversos (pasada 1) van girados
	if a := strings.Join(m.argumentos(1), " "); !strings.Contains(a, "UpsideDown") {
		t.Errorf("pasada 1 con giro de calendario: %s", a)
	}
	if a := strings.Join(m.argumentos(2), " "); !strings.Contains(a, "Portrait") {
		t.Errorf("la pasada 2 (frentes) nunca se gira: %s", a)
	}
	m.cfg.Giro, m.revInvertir = "libro", true // la tecla g
	if a := strings.Join(m.argumentos(1), " "); !strings.Contains(a, "UpsideDown") {
		t.Errorf("g debe girar la tanda de reversos: %s", a)
	}
	m.revInvertir, m.iOrient = false, 1 // apaisado + libro
	if a := strings.Join(m.argumentos(1), " "); !strings.Contains(a, "Seascape") {
		t.Errorf("apaisado: %s", a)
	}
	if a := strings.Join(m.argumentos(2), " "); !strings.Contains(a, "StpOrientation=Landscape") {
		t.Errorf("los frentes no se giran: %s", a)
	}
}

func TestPasadasEnElOrdenQueDejaLaPagina1Arriba(t *testing.T) {
	m := modeloBase(t)
	m.caras, m.copias = 1, 1
	// sin PDF reordenado, cae a rangos ascendentes (nunca imprime todo el documento)
	a1 := strings.Join(m.argumentos(1), " ")
	if !strings.Contains(a1, "page-ranges=2,4") {
		t.Errorf("respaldo de la pasada 1: %s", a1)
	}
	m.pdfPasada[1], m.pdfPasada[2] = "/c/p1.pdf", "/c/p2.pdf"
	for p, f := range map[int]string{1: "/c/p1.pdf", 2: "/c/p2.pdf"} {
		a := strings.Join(m.argumentos(p), " ")
		if !strings.HasSuffix(a, "-- "+f) || strings.Contains(a, "page-ranges") {
			t.Errorf("pasada %d debe imprimir su PDF reordenado entero: %s", p, a)
		}
	}
	v := m.lineasResumen(1)
	if !strings.Contains(strings.Join(v, "|"), "4, 2") {
		t.Errorf("la pasada 1 debe listar 4, 2: %v", v)
	}
	if !strings.Contains(strings.Join(m.lineasResumen(2), "|"), "3, 1") {
		t.Errorf("la pasada 2 debe listar 3, 1: %v", m.lineasResumen(2))
	}
}

func TestTeclaGInvierteElReversoYSeGuarda(t *testing.T) {
	m := modeloBase(t)
	m.pant, m.caras = pVolteo, 1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !m.revInvertir {
		t.Fatal("g debe invertir")
	}
	if s := CargarSesionSinFiltro(t); s == nil || s.Opciones["revinv"] != "1" {
		t.Errorf("la sesión debe recordarlo: %+v", s)
	}
	if v := m.View(); !strings.Contains(v, "g girar reverso") {
		t.Error("el pie debe mencionar la tecla g")
	}
}

// CargarSesionSinFiltro lee sesion.json sin las comprobaciones de CargarSesion
// (el archivo de prueba no existe de verdad).
func CargarSesionSinFiltro(t *testing.T) *Sesion {
	t.Helper()
	b, err := os.ReadFile(rutaSesion())
	if err != nil {
		return nil
	}
	var s Sesion
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

func TestImparConHojaEnBlancoAutomaticaNoPideNada(t *testing.T) {
	m := modeloBase(t)
	m.caras, m.copias, m.info.Paginas = 1, 1, 5
	m.pant, m.plan = pVolteo, PlanificarDuplex([]int{1, 2, 3, 4, 5})
	m.blancoAuto = false
	if v := m.View(); !strings.Contains(v, "hoja en blanco encima") {
		t.Error("sin la hoja automática hay que pedirla")
	}
	m.blancoAuto = true
	if v := m.View(); strings.Contains(v, "hoja en blanco encima") {
		t.Error("con la hoja automática no hay que pedir nada")
	}
	m.pant = pVista
	if l := strings.Join(m.lineasResumen(1), "|"); !strings.Contains(l, "hoja en blanco, 4, 2") {
		t.Errorf("la pasada 1 debe mostrar la hoja en blanco: %s", l)
	}
	if l := strings.Join(m.lineasResumen(2), "|"); !strings.Contains(l, "5, 3, 1") {
		t.Errorf("la pasada 2: %s", l)
	}
}

func TestBannerNoAvisaDelTrabajoQueYaSeImprime(t *testing.T) {
	m := modeloBase(t)
	m.trabajosCola = []Trabajo{{ID: "a-1", Propio: true, Estado: "imprimiendo"}}
	if b := strings.Join(m.banda(), "\n"); strings.Contains(b, "en cola") {
		t.Errorf("un trabajo ya imprimiéndose no necesita aviso: %q", b)
	}
	m.trabajosCola = append(m.trabajosCola, Trabajo{ID: "a-2", Propio: true, Estado: "retenido", EdadS: 600})
	if b := strings.Join(m.banda(), "\n"); !strings.Contains(b, "1 trabajo en cola") {
		t.Errorf("el que espera sí: %q", b)
	}
}

func TestEscalaMuestraDosYElRestoEnMasOpciones(t *testing.T) {
	m := modeloBase(t)
	m.info = InfoPDF{Paginas: 1, Ancho: 595, Alto: 842, Papel: "A4"} // A4 en Carta: hay desajuste
	m.fila = rEscala
	f := m.filaEscala()
	for _, quiero := range []string{"Ajustar", "Tamaño real", "Más"} {
		if !strings.Contains(f, quiero) {
			t.Errorf("falta %q en %q", quiero, f)
		}
	}
	for _, noQuiero := range []string{"Estirar", "Cubrir"} {
		if strings.Contains(f, noQuiero) {
			t.Errorf("%q debe estar escondida: %q", noQuiero, f)
		}
	}
	tecla := func(k tea.KeyType) { m.Update(tea.KeyMsg{Type: k}) }
	tecla(tea.KeyLeft) // en el primero, a la izquierda no hay nada
	if m.iEscala != 0 || m.escalaAbierta {
		t.Fatal("a la izquierda del primero no debe dar la vuelta")
	}
	tecla(tea.KeyRight)
	if m.iEscala != 1 {
		t.Fatalf("→ elige Tamaño real: %d", m.iEscala)
	}
	tecla(tea.KeyRight) // «Más»
	if !m.escalaAbierta {
		t.Fatal("llegar a «Más» abre el selector")
	}
	if f := m.filaEscala(); !strings.Contains(f, "Tamaño real") || !strings.Contains(f, "Enter confirmar") {
		t.Errorf("selector: %q", f)
	}
	tecla(tea.KeyRight) // Estirar
	tecla(tea.KeyEnter)
	if m.escalaAbierta || m.iEscala != 2 || len(m.escalaVis) != 3 {
		t.Fatalf("Estirar debe ocupar el tercer lugar: abierta=%v i=%d vis=%v", m.escalaAbierta, m.iEscala, m.escalaVis)
	}
	if f := m.filaEscala(); !strings.Contains(f, "Estirar") || !strings.Contains(f, "Más") || strings.Contains(f, "Cubrir") {
		t.Errorf("fila tras elegir: %q", f)
	}
	if m.escalaArg() != "" {
		t.Error("estirar no usa print-scaling")
	}
	// volver atrás con ←
	tecla(tea.KeyLeft)
	if m.iEscala != 1 {
		t.Errorf("← vuelve a Tamaño real: %d", m.iEscala)
	}
	// elegir Cubrir reemplaza al tercer lugar y esc cancela sin cambiar
	tecla(tea.KeyRight)
	tecla(tea.KeyRight)
	tecla(tea.KeyRight) // «Más» (estaba en el 3.º: Estirar -> Más)
	if !m.escalaAbierta {
		t.Fatal("debe abrir")
	}
	tecla(tea.KeyRight)
	tecla(tea.KeyEnter)
	if m.iEscala != 3 || m.escalasVisibles()[2] != 3 {
		t.Errorf("Cubrir reemplaza al tercero: i=%d vis=%v", m.iEscala, m.escalaVis)
	}
	if m.escalaArg() != "fill" {
		t.Errorf("cubrir = fill: %q", m.escalaArg())
	}
	tecla(tea.KeyRight)
	tecla(tea.KeyEsc)
	if m.escalaAbierta || m.iEscala != 3 {
		t.Errorf("esc cancela sin cambiar: abierta=%v i=%d", m.escalaAbierta, m.iEscala)
	}
}

func TestCabeceraDelMenuEsNeutra(t *testing.T) {
	m := modeloBase(t)
	_, lineas, _ := m.vMenu(80)
	texto := strings.Join(lineas, "\n")
	if !strings.Contains(texto, "Impresora") {
		t.Errorf("falta la etiqueta neutra:\n%s", texto)
	}
	if strings.Contains(texto, "Canon_G2110") {
		t.Errorf("la cabecera no debe mostrar la cola:\n%s", texto)
	}
}

func TestPantallaListoCuentaLoImpresoSinCodigos(t *testing.T) {
	m := modeloBase(t)
	m.pant, m.jobs, m.job = pListo, []string{"Canon_G2110-47"}, "Canon_G2110-47"
	m.caras, m.copias, m.info.Paginas = 1, 1, 4
	v := m.View()
	for _, quiero := range []string{"informe.pdf", "todas (4)", "2 (doble cara)", "Carta", "Normal"} {
		if !strings.Contains(v, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, v)
		}
	}
	for _, noQuiero := range []string{"Canon_G2110-47", "sudo cancel", "Trabajo(s)", "CUPS"} {
		if strings.Contains(v, noQuiero) {
			t.Errorf("no debe mostrar %q", noQuiero)
		}
	}
	m.caras, m.modoRango, m.rangoTxt, m.copias = 0, true, "1-3", 2
	v = m.View()
	if !strings.Contains(v, "1-3") || !strings.Contains(v, "6 (una cara)") || !strings.Contains(v, "Copias") {
		t.Errorf("rango y copias:\n%s", v)
	}
}

func TestPantallaImprimiendoSinCodigos(t *testing.T) {
	m := modeloBase(t)
	m.pant, m.job, m.textoJob, m.caras, m.pasada = pImprimiendo, "Canon_G2110-47", "Imprimiendo", 1, 1
	v := m.View()
	if strings.Contains(v, "Canon_G2110-47") || !strings.Contains(v, "informe.pdf") || !strings.Contains(v, "pasada 1 de 2") {
		t.Errorf("imprimiendo:\n%s", v)
	}
}
