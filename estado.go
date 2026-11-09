package main

import (
	"context"
	_ "embed"
	"encoding/csv"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed ipp_jobs.test
var ippJobsTest []byte

// Trabajo es un trabajo de la cola de CUPS.
type Trabajo struct {
	ID      string    `json:"id"`
	Nombre  string    `json:"nombre"`
	Usuario string    `json:"usuario"`
	Bytes   int64     `json:"bytes"`
	Creado  time.Time `json:"creado"`
	EdadS   int64     `json:"edad_s"` // -1 si no se pudo saber
	Estado  string    `json:"estado"` // pendiente | retenido | imprimiendo | detenido
	Origen  string    `json:"origen"` // print | otro
	Propio  bool      `json:"propio"`
}

type Alerta struct {
	Nivel  string `json:"nivel"` // aviso | error
	Codigo string `json:"codigo"`
	Texto  string `json:"texto"`
}

// Estado es lo que consumen el icono de la barra y el menú (spec indicador §3.1).
type Estado struct {
	V          int         `json:"v"`
	TS         time.Time   `json:"ts"`
	Cola       string      `json:"cola"`
	Nombre     string      `json:"nombre"` // nombre legible que declara CUPS
	Estado     string      `json:"estado"` // sin_cups | sin_cola | desconectada | detenida | lista | imprimiendo | espera | error
	USB        bool        `json:"usb"`
	Habilitada bool        `json:"habilitada"`
	Aceptando  bool        `json:"aceptando"`
	Razon      string      `json:"razon"`
	Trabajos   []Trabajo   `json:"trabajos"`
	Alertas    []Alerta    `json:"alertas"`
	Impresoras []Impresora `json:"impresoras"` // todas las colas configuradas
	Tinta      any         `json:"tinta"`      // CUPS no informa niveles en esta impresora
}

// Impresora es una cola configurada en CUPS, con lo justo para listarla.
type Impresora struct {
	Cola           string `json:"cola"`
	Nombre         string `json:"nombre"`
	Predeterminada bool   `json:"predeterminada"`
	Conectada      bool   `json:"conectada"`
	Estado         string `json:"estado"` // lista | desconectada
}

// LeerImpresoras lista las colas que CUPS conoce y si su dispositivo está
// presente ahora. Así el panel puede recordar las impresoras usadas antes.
func LeerImpresoras(ctx context.Context, predeterminada string) []Impresora {
	out, _, err := ejecutar(ctx, 3*time.Second, "lpstat", "-v")
	if err != nil {
		return nil
	}
	var res []Impresora
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "device for ") {
			continue
		}
		resto := strings.TrimPrefix(l, "device for ")
		i := strings.Index(resto, ": ")
		if i < 0 {
			continue
		}
		cola, uri := resto[:i], resto[i+2:]
		conectada := conectadaPorURI(uri)
		est := "lista"
		if !conectada {
			est = "desconectada"
		}
		res = append(res, Impresora{
			Cola:           cola,
			Nombre:         nombreImpresora(ctx, cola),
			Predeterminada: cola == predeterminada,
			Conectada:      conectada,
			Estado:         est,
		})
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Predeterminada != res[j].Predeterminada {
			return res[i].Predeterminada
		}
		if res[i].Conectada != res[j].Conectada {
			return res[i].Conectada
		}
		return res[i].Nombre < res[j].Nombre
	})
	return res
}

const umbralViejoS = 5 * 60

var reIDTrabajo = regexp.MustCompile(`^[A-Za-z0-9_.-]+-[0-9]+$`)
var reColaValida = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// motivos de printer-state-reasons que son un problema que el usuario resuelve.
var motivosError = map[string]string{
	"media-empty":         "Sin papel",
	"media-needed":        "Falta papel",
	"media-jam":           "Atasco de papel",
	"cover-open":          "Tapa abierta",
	"door-open":           "Tapa abierta",
	"marker-supply-empty": "Tinta agotada",
	"toner-empty":         "Tinta agotada",
	"output-area-full":    "Bandeja de salida llena",
}

var motivosAviso = map[string]string{
	"marker-supply-low": "Tinta baja",
	"toner-low":         "Tinta baja",
}

// LeerEstado junta CUPS, USB y cola en una sola foto. Nunca entra en pánico:
// lo que no se pueda saber se deja vacío.
func LeerEstado(ctx context.Context, cola string) Estado {
	e := Estado{V: 1, TS: time.Now(), Cola: cola, Trabajos: []Trabajo{}, Alertas: []Alerta{}, Impresoras: []Impresora{}}
	if !existe("lpstat") {
		e.Estado = "sin_cups"
		return e
	}
	if _, _, err := ejecutar(ctx, 3*time.Second, "lpstat", "-r"); err != nil {
		e.Estado = "sin_cups"
		return e
	}
	e.Impresoras = LeerImpresoras(ctx, cola)
	c, err := LeerCola(ctx, cola)
	if err != nil || !c.Existe {
		e.Estado = "sin_cola"
		return e
	}
	e.Habilitada, e.Aceptando, e.Razon = c.Habilitada, c.Aceptando, c.Razon
	e.Nombre = nombreImpresora(ctx, cola)
	e.USB = impresoraConectada(ctx, cola)
	e.Trabajos = LeerTrabajos(ctx, cola)
	if e.Trabajos == nil {
		e.Trabajos = []Trabajo{} // JSON [] y no null
	}
	razones := leerRazones(ctx, cola)

	for _, r := range razones {
		if t, ok := motivosError[r]; ok {
			e.Alertas = append(e.Alertas, Alerta{"error", r, t})
		} else if t, ok := motivosAviso[r]; ok {
			e.Alertas = append(e.Alertas, Alerta{"aviso", r, t})
		}
	}
	if !e.USB {
		e.Alertas = append(e.Alertas, Alerta{"aviso", "P-6", "Impresora desconectada o apagada"})
	}
	if !e.Habilitada && e.USB { // sin USB ya lo explica «desconectada»
		e.Alertas = append(e.Alertas, Alerta{"aviso", "P-4", "La cola está detenida" + razon(e.Razon)})
	}
	if !e.Aceptando {
		e.Alertas = append(e.Alertas, Alerta{"aviso", "P-5", "La cola no acepta trabajos"})
	}
	viejos := 0
	for _, t := range e.Trabajos {
		if t.EdadS > umbralViejoS {
			viejos++
		}
	}
	if viejos > 0 {
		e.Alertas = append(e.Alertas, Alerta{"aviso", "J-1", pluralTrabajos(viejos) + " en cola con más de 5 minutos"})
	}

	switch {
	case !e.USB:
		e.Estado = "desconectada"
	case !e.Habilitada || !e.Aceptando:
		e.Estado = "detenida"
	case tieneAlerta(e.Alertas, "error"):
		e.Estado = "error"
	case hayEstado(e.Trabajos, "imprimiendo"):
		e.Estado = "imprimiendo"
	case len(e.Trabajos) > 0:
		e.Estado = "espera"
	default:
		e.Estado = "lista"
	}
	return e
}

func pluralTrabajos(n int) string {
	if n == 1 {
		return "1 trabajo"
	}
	return strconv.Itoa(n) + " trabajos"
}

func tieneAlerta(a []Alerta, nivel string) bool {
	for _, x := range a {
		if x.Nivel == nivel {
			return true
		}
	}
	return false
}

func hayEstado(t []Trabajo, est string) bool {
	for _, x := range t {
		if x.Estado == est {
			return true
		}
	}
	return false
}

// leerRazones extrae «Alerts:» de lpstat -l -p (printer-state-reasons).
func leerRazones(ctx context.Context, cola string) []string {
	out, _, _ := ejecutar(ctx, 3*time.Second, "lpstat", "-l", "-p", cola)
	var rs []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if v, ok := strings.CutPrefix(l, "Alerts:"); ok {
			for _, r := range strings.Fields(strings.ReplaceAll(v, ",", " ")) {
				r = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(r, "-report"), "-warning"), "-error")
				rs = append(rs, r)
			}
		}
	}
	return rs
}

// LeerTrabajos pide los trabajos pendientes por IPP (con nombre) y, si ipptool
// no está o falla, cae a lpstat (solo ID).
func LeerTrabajos(ctx context.Context, cola string) []Trabajo {
	if t, ok := trabajosIPP(ctx, cola); ok {
		return marcarOrigen(t)
	}
	return marcarOrigen(trabajosLpstat(ctx, cola))
}

func miUsuario() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

func marcarOrigen(ts []Trabajo) []Trabajo {
	reg := LeerRegistro()
	yo := miUsuario()
	ahora := time.Now()
	for i := range ts {
		ts[i].Propio = ts[i].Usuario == yo
		ts[i].Origen = "otro"
		if r, ok := reg[ts[i].ID]; ok {
			ts[i].Origen = "print"
			if r.Archivo != "" {
				ts[i].Nombre = r.Archivo
			}
		}
		if !ts[i].Creado.IsZero() {
			ts[i].EdadS = int64(ahora.Sub(ts[i].Creado).Seconds())
		} else {
			ts[i].EdadS = -1
		}
	}
	return ts
}

var estadoIPP = map[string]string{
	"pending": "pendiente", "pending-held": "retenido",
	"processing": "imprimiendo", "processing-stopped": "detenido",
}

func trabajosIPP(ctx context.Context, cola string) ([]Trabajo, bool) {
	if !existe("ipptool") || !reColaValida.MatchString(cola) {
		return nil, false
	}
	f, err := os.CreateTemp("", "print-jobs-*.test")
	if err != nil {
		return nil, false
	}
	defer os.Remove(f.Name())
	f.Write(ippJobsTest)
	f.Close()
	out, _, err := ejecutar(ctx, 3*time.Second, "ipptool", "-c", "-d", "which=not-completed", "ipp://localhost/printers/"+cola, f.Name())
	if err != nil {
		return nil, false
	}
	filas, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil || len(filas) == 0 {
		return nil, false
	}
	var ts []Trabajo
	for _, r := range filas[1:] { // job-id,job-name,job-state,job-state-reasons,user,k-octets,time-at-creation
		if len(r) < 7 {
			continue
		}
		est, ok := estadoIPP[r[2]]
		if !ok {
			continue // terminado o cancelado
		}
		n, _ := strconv.ParseInt(r[5], 10, 64)
		sec, _ := strconv.ParseInt(r[6], 10, 64)
		t := Trabajo{ID: cola + "-" + r[0], Nombre: r[1], Usuario: r[4], Bytes: n * 1024, Estado: est}
		if sec > 0 {
			t.Creado = time.Unix(sec, 0)
		}
		ts = append(ts, t)
	}
	return ts, true
}

var reLpstatTrabajo = regexp.MustCompile(`^(\S+-\d+)\s+(\S+)\s+(\d+)`)

func trabajosLpstat(ctx context.Context, cola string) []Trabajo {
	out, _, _ := ejecutar(ctx, 3*time.Second, "lpstat", "-W", "not-completed", "-o", cola)
	var ts []Trabajo
	for _, l := range strings.Split(out, "\n") {
		if m := reLpstatTrabajo.FindStringSubmatch(l); m != nil {
			b, _ := strconv.ParseInt(m[3], 10, 64)
			ts = append(ts, Trabajo{ID: m[1], Nombre: m[1], Usuario: m[2], Bytes: b, Estado: "pendiente"})
		}
	}
	return ts
}

// NombreCorto devuelve el archivo sin ruta, para mostrarlo.
func (t Trabajo) NombreCorto() string { return filepath.Base(t.Nombre) }

// EstadoFinal dice cómo terminó un trabajo que ya no está pendiente:
// "completed", "canceled", "aborted" o "" si no se puede saber. Sirve para no
// avanzar a la pantalla de volteo cuando la pasada 1 en realidad falló.
func EstadoFinal(ctx context.Context, cola, id string) string {
	if !existe("ipptool") || !reColaValida.MatchString(cola) || !reIDTrabajo.MatchString(id) {
		return ""
	}
	n := id[strings.LastIndex(id, "-")+1:]
	f, err := os.CreateTemp("", "print-final-*.test")
	if err != nil {
		return ""
	}
	defer os.Remove(f.Name())
	f.Write(ippJobsTest)
	f.Close()
	out, _, err := ejecutar(ctx, 3*time.Second, "ipptool", "-c", "-d", "which=completed", "ipp://localhost/printers/"+cola, f.Name())
	if err != nil {
		return ""
	}
	filas, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		return ""
	}
	for _, r := range filas[min(1, len(filas)):] {
		if len(r) >= 3 && r[0] == n {
			return r[2]
		}
	}
	return ""
}
