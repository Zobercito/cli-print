package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

//go:embed opciones_verificadas.json
var opcionesJSON []byte

type pantalla int

const (
	pArranque pantalla = iota
	pReanudar
	pBuscador
	pAnalizando
	pMenu
	pVista
	pImprimiendo
	pVolteo
	pListo
	pError
)

// filas del menú
const (
	rTinta = iota
	rCalidad
	rPapel
	rEscala
	rCaras
	rPaginas
	rCopias
	rOrient
	rMedio
	rBordes
	nFilas
)

type par struct{ Val, Txt string }

var (
	tintas    = []par{{"normal", "Normal"}, {"gris", "B/N"}, {"color", "Solo color"}}
	calidades = []par{{"borrador", "Borrador"}, {"normal", "Normal"}, {"alta", "Alta"}}
	orients   = []par{{"Portrait", "Vertical"}, {"Landscape", "Horizontal"}}
	medios    = []par{{"", "Normal"}, {"PhotopaperMatte", "Mate"}, {"PhotoPlusGloss2", "Brillante"}}
)

type (
	tickMsg time.Time
	msgPre  struct {
		cola     string
		hs       []Hallazgo
		trabajos []Trabajo
	}
	msgAuto struct {
		cola     string
		hs       []Hallazgo
		trabajos []Trabajo
	}
	msgCancelado struct {
		errs   []error
		pasada bool // se canceló el trabajo en curso desde la pantalla de impresión
	}
	msgHabilitado struct{ err error }
	msgArchivos   []Archivo
	msgAnalizado  struct {
		id   int
		pdf  string
		info InfoPDF
		err  error
	}
	msgEnviado struct {
		id  string
		err error
	}
	msgPoll struct {
		job   string
		st    EstadoTrabajo
		cola  EstadoCola
		final string // cómo terminó: completed | canceled | aborted | ""
	}
)

type model struct {
	ctx     context.Context
	cfg     Config
	sim     Simbolos
	anim    bool
	dry     bool
	w, h    int
	pant    pantalla
	frame   int
	entrada time.Time
	shake   time.Time

	cola       string
	nombre     string // nombre legible de la impresora (solo para el detalle)
	hallazgos  []Hallazgo
	descartado bool
	ses        *Sesion

	// buscador
	argRuta  string
	archivos []Archivo
	busq     textinput.Model
	coin     []Coincidencia
	cur      int

	// documento
	ruta, pdf string
	tam       int64
	info      InfoPDF
	analID    int
	cancel    context.CancelFunc

	// menú
	fila                   int
	iTinta, iCalidad       int
	caras, copias          int
	papel                  string
	papelVis, todosPapeles []string
	iEscala                int
	escalaVis              []int // opciones de escala visibles (las demás están en «Más»)
	escalaCur, escalaPick  int
	escalaAbierta          bool
	escalaForzada, pdfImp  string
	papelCur, papelPick    int
	papelAbierto           bool
	iOrient, iMedio        int
	bordes, modoRango      bool
	rangoTxt               string
	extra                  []string
	avisoCMY               bool
	pulso                  time.Time

	// vista previa
	editando bool
	edit     textinput.Model
	editErr  string

	// impresión
	plan          PlanDuplex
	copia, pasada int
	job           string
	jobs          []string
	desconocido   int
	inicioJob     time.Time
	textoJob      string

	recargando    bool
	verOrden      bool
	pdfPasada     [3]string // PDF reordenado de cada pasada del doble cara
	enviandoP2    bool
	blancoAuto    bool      // la hoja extra de un total impar la añade print, no el usuario
	revInvertir   bool      // `g` en el volteo: girar 180° el reverso respecto al valor por defecto
	trabajosCola  []Trabajo // lo que hay en la cola de CUPS (spec indicador, fase 6)
	confirmCancel bool
	autoUlt       time.Time
	autoBusy      bool
	dupAviso      string
	dupOK         bool
	nota          string
	retryPasada   int
	fin, dejar    bool // al salir: borrar el menú, salvo la pantalla «Listo»
	err           *Mensaje
	errOrig       string
	volver        pantalla
	salida        string
	msgCopia      string
}

func nuevoModelo(ctx context.Context, cfg Config, ruta string, dry bool, anim bool, ascii bool) *model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Width = 50
	ti.Placeholder = "escribe para buscar…"
	ti.Focus()
	ed := textinput.New()
	ed.Prompt = "-o "
	ed.Width = 50
	ed.Placeholder = "Clave=Valor …"
	m := &model{ctx: ctx, cfg: cfg, sim: elegirSimbolos(ascii), anim: anim, dry: dry,
		w: 80, h: 30, pant: pArranque, entrada: time.Now(), cola: cfg.Impresora,
		nombre: nombreImpresora(ctx, cfg.Impresora),
		busq:   ti, edit: ed, argRuta: ruta,
		copias: 1, pasada: 1, copia: 1, papel: "Letter",
		papelVis: []string{"Letter", "A4"}, todosPapeles: TamanosPapel(cfg.Impresora)}
	if p := papelDeCola(ctx, cfg.Impresora); p != "" {
		m.fijarPapel(p)
	}
	for i, t := range tintas {
		if t.Val == cfg.Tinta {
			m.iTinta = i
		}
	}
	for i, c := range calidades {
		if c.Val == cfg.Calidad {
			m.iCalidad = i
		}
	}
	return m
}

func (m *model) setPant(p pantalla) {
	m.pant, m.entrada = p, time.Now()
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.tick(), cmdPreflight(m.ctx, m.cola, true))
}

func (m *model) tick() tea.Cmd {
	d := 60 * time.Millisecond
	if !m.anim {
		d = 500 * time.Millisecond
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ---------- comandos ----------

func cmdPreflight(ctx context.Context, cola string, pausa bool) tea.Cmd {
	return func() tea.Msg {
		c, hs := Preflight(ctx, cola)
		tr := LeerTrabajos(ctx, c)
		if pausa {
			time.Sleep(350 * time.Millisecond) // que el spinner de arranque se vea
		}
		return msgPre{c, hs, tr}
	}
}

// cmdAuto repite el chequeo en segundo plano sin tocar la pantalla actual.
func cmdAuto(ctx context.Context, cola string) tea.Cmd {
	return func() tea.Msg {
		c, hs := Preflight(ctx, cola)
		return msgAuto{c, hs, LeerTrabajos(ctx, c)}
	}
}

func cmdCancelarTodos(ctx context.Context, cola string, pasada bool) tea.Cmd {
	return func() tea.Msg { return msgCancelado{CancelarTodos(ctx, cola), pasada} }
}

func cmdCancelarUno(ctx context.Context, id string) tea.Cmd {
	return func() tea.Msg {
		if err := Cancelar(ctx, id); err != nil && err != errYaNoExiste {
			return msgCancelado{[]error{err}, true}
		}
		return msgCancelado{nil, true}
	}
}

func cmdHabilitar(cola string) tea.Cmd {
	exe, err := os.Executable()
	if err != nil {
		exe = "print"
	}
	c := exec.Command(exe, "habilitar", "--cola", cola)
	return tea.ExecProcess(c, func(err error) tea.Msg { return msgHabilitado{err} })
}

// cmdArchivos lista los documentos de una sola carpeta: la indicada o, si no
// hay, la carpeta actual. No recorre subcarpetas ni busca en otras ubicaciones:
// si quieres imprimir algo de otro sitio, ve a esa carpeta y ejecuta print.
func cmdArchivos(ctx context.Context, arg string) tea.Cmd {
	return func() tea.Msg {
		dir := arg
		if dir == "" {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return msgArchivos(nil)
			}
		}
		// También con argumentos relativos (print ., print sub/): así la columna
		// de carpeta muestra la ruta real y no ".".
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		return msgArchivos(ListarDocumentos([]string{dir}, 0))
	}
}

func cmdAnalizar(ctx context.Context, id int, ruta string, to time.Duration) tea.Cmd {
	return func() tea.Msg {
		pdf, err := Convertir(ctx, ruta, to)
		if err != nil {
			return msgAnalizado{id: id, err: err}
		}
		info, err := AnalizarPDF(ctx, pdf)
		return msgAnalizado{id, pdf, info, err}
	}
}

func cmdEnviar(ctx context.Context, args []string, dry bool, archivo string) tea.Cmd {
	return func() tea.Msg {
		if dry {
			return msgEnviado{id: "dry-run"}
		}
		id, err := Enviar(ctx, args)
		if err == nil {
			RegistrarEnvio(id, filepath.Base(archivo))
		}
		return msgEnviado{id, err}
	}
}

func cmdPoll(ctx context.Context, cola, job string, dry bool, espera time.Duration) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(espera):
		}
		if dry {
			return msgPoll{job: job, st: TrabajoTerminado, final: "completed"}
		}
		st := ConsultarTrabajo(ctx, cola, job)
		var c EstadoCola
		final := ""
		if st == TrabajoActivo {
			c, _ = LeerCola(ctx, cola)
		} else {
			final = EstadoFinal(ctx, cola, job)
		}
		return msgPoll{job, st, c, final}
	}
}

func copiarPortapapeles(s string) bool {
	for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}} {
		if existe(c[0]) {
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(s)
			return cmd.Run() == nil
		}
	}
	return false
}

// ---------- lógica de opciones ----------

func (m *model) seleccion() ([]int, error) {
	if !m.modoRango {
		var s []int
		for i := 1; i <= m.info.Paginas; i++ {
			s = append(s, i)
		}
		return s, nil
	}
	if strings.TrimSpace(m.rangoTxt) == "" {
		return nil, fmt.Errorf("escribe un rango, por ejemplo 1-3,7")
	}
	return ParseRangos(m.rangoTxt, m.info.Paginas)
}

func (m *model) opciones(paginas []int, orient string) Opciones {
	o := Opciones{Cola: m.cola, Papel: m.papel, Tinta: tintas[m.iTinta].Val,
		Calidad:     calidades[m.iCalidad].Val,
		Orientacion: orient, Copias: 1, Medio: medios[m.iMedio].Val, SinBordes: m.bordes,
		Extra: m.extra, Paginas: paginas, Escala: m.escalaArg()}
	return o
}

func (m *model) usaDuplex() bool { return m.caras == 1 }

// planes devuelve los argumentos de lp de cada pasada (para vista previa e impresión).
func (m *model) argumentos(pasada int) []string {
	sel, _ := m.seleccion()
	or := orients[m.iOrient].Val
	if !m.usaDuplex() {
		var pag []int
		if m.modoRango {
			pag = sel
		}
		o := m.opciones(pag, or)
		o.Copias = m.copias
		o.Orden = "reverse" // la última en salir queda encima: la página 1 arriba
		return ConstruirLP(o, m.archivoImp())
	}
	pl := PlanificarDuplex(sel)
	if pasada == 1 { // los reversos van primero; si hay que girarlos, es esta tanda
		if m.reversoRotado() {
			or = OrientacionReverso(or)
		}
		return m.argsPasada(1, pl.Pasada1, or)
	}
	return m.argsPasada(2, pl.Pasada2, or)
}

func (m *model) problemaImprimir() string {
	for _, h := range m.hallazgos {
		if h.Grave || h.Bloqueante {
			return h.Que
		}
	}
	return m.problemaSeleccion()
}

// problemaSeleccion: lo que impide imprimir por culpa de las opciones elegidas.
func (m *model) problemaSeleccion() string {
	if _, err := m.seleccion(); err != nil {
		return err.Error()
	}
	if m.usaDuplex() {
		if m.info.Paginas == 0 {
			return "doble cara necesita saber el total de páginas"
		}
		sel, _ := m.seleccion()
		if !DuplexUtil(sel) {
			return "el doble cara necesita al menos 2 páginas"
		}
	}
	return ""
}

func (m *model) sesion(estado string) *Sesion {
	sel, _ := m.seleccion()
	pl := PlanificarDuplex(sel)
	s := &Sesion{ID: time.Now().Format("2006-01-02T15:04:05"), Archivo: m.ruta, PDF: m.archivoImp(), Paginas: m.info.Paginas,
		A: pl.A, R: pl.R, Estado: estado, Job1: m.job, Copia: m.copia, Copias: m.copias,
		Opciones: map[string]string{"cola": m.cola, "papel": m.papel, "tinta": tintas[m.iTinta].Val,
			"calidad": calidades[m.iCalidad].Val,
			"orient":  orients[m.iOrient].Val, "medio": medios[m.iMedio].Val, "escala": m.escalaArg(), "bordes": b2s(m.bordes), "extra": strings.Join(m.extra, " "), "revinv": b2s(m.revInvertir)}}
	if st, err := os.Stat(m.ruta); err == nil {
		s.Mtime = st.ModTime().Unix()
	}
	return s
}

func b2s(b bool) string {
	if b {
		return "1"
	}
	return ""
}

// Las que el menú ya controla no se pueden pisar desde «editar opciones»
// (si no, el reverso del doble cara podría perder su giro, por ejemplo).
var gestionadas = map[string]bool{"ColorModel": true, "PageSize": true, "StpOrientation": true,
	"MediaType": true, "StpFullBleed": true, "page-ranges": true, "copies": true}

func clavesPermitidas() map[string]bool {
	var v struct {
		Opciones map[string]struct{ Estado string } `json:"opciones"`
	}
	json.Unmarshal(opcionesJSON, &v)
	out := map[string]bool{}
	for k, o := range v.Opciones {
		if o.Estado == "real" && !gestionadas[k] {
			out[k] = true
		}
	}
	return out
}

func claves() string {
	var k []string
	for c := range clavesPermitidas() {
		k = append(k, c)
	}
	sort.Strings(k)
	return strings.Join(k, " ")
}

// reversoRotado dice si el reverso se imprime girado 180° respecto al frente.
// Con el giro de libro (por un borde largo) el borde de arriba de la página es
// el mismo en las dos caras, así que en vertical no hace falta girarlo; con el
// giro de calendario (por un borde corto) sí. Con el contenido apaisado se
// invierte. `g` en la pantalla de volteo lo cambia para calibrar con papel real.
func (m *model) reversoRotado() bool {
	base := m.cfg.Giro == "tortita"
	if m.iOrient == 1 || m.apaisado() {
		base = !base
	}
	return base != m.revInvertir
}

// argsPasada usa el PDF ya reordenado de la pasada. Si no se preparó (no debería
// pasar) cae a un rango ascendente en vez de imprimir el documento entero.
func (m *model) argsPasada(n int, paginas []int, orient string) []string {
	// El PDF de cada pasada ya viene en el orden real de impresión
	// (PrepararPasadas), así que CUPS no debe invertirlo otra vez.
	if f := m.pdfPasada[n]; f != "" {
		o := m.opciones(nil, orient)
		o.Orden = "normal"
		return ConstruirLP(o, f)
	}
	asc := append([]int(nil), paginas...)
	sort.Ints(asc)
	o := m.opciones(asc, orient)
	o.Orden = "normal"
	return ConstruirLP(o, m.archivoImp())
}

// prepararDuplex crea los PDF reordenados de las dos pasadas.
func (m *model) prepararDuplex() error {
	m.pdfPasada, m.blancoAuto = [3]string{}, false
	if !m.usaDuplex() {
		return nil
	}
	sel, _ := m.seleccion()
	p1, p2, blanco, err := PrepararPasadas(m.ctx, m.archivoImp(), PlanificarDuplex(sel))
	if err != nil {
		return err
	}
	m.pdfPasada[1], m.pdfPasada[2], m.blancoAuto = p1, p2, blanco
	return nil
}
