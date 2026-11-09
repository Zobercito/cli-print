package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Detalles es la ficha de una cola para el panel: lo que CUPS sabe de ella y
// que no cabe en el estado resumido.
type Detalles struct {
	Cola           string            `json:"cola"`
	Nombre         string            `json:"nombre"`
	URI            string            `json:"uri"`
	Modelo         string            `json:"modelo"`
	Ubicacion      string            `json:"ubicacion"`
	Estado         string            `json:"estado"`
	Razon          string            `json:"razon"`
	Habilitada     bool              `json:"habilitada"`
	Aceptando      bool              `json:"aceptando"`
	Cups           bool              `json:"cups"`
	Predeterminada bool              `json:"predeterminada"`
	Alertas        []Alerta          `json:"alertas"`
	Opciones       map[string]string `json:"opciones"`
}

// paginaPrueba es una variable para que las pruebas no dependan del archivo
// real que instala CUPS.
var paginaPrueba = "/usr/share/cups/data/testprint"

// LeerDetalles consulta la cola sin imprimir nada. Si CUPS no responde,
// devuelve Cups=false y el panel muestra el resto de lo que haya podido leer.
func LeerDetalles(ctx context.Context, cola string) Detalles {
	d := Detalles{Cola: cola, Alertas: []Alerta{}, Opciones: map[string]string{}}
	if !existe("lpstat") {
		return d
	}
	if _, _, err := ejecutar(ctx, 3*time.Second, "lpstat", "-r"); err != nil {
		return d
	}
	d.Cups = true
	d.Predeterminada = ColaPorDefecto() == cola
	d.URI = uriDispositivo(ctx, cola)
	d.Nombre = nombreImpresora(ctx, cola)
	d.Opciones = leerOpciones(ctx, cola)
	d.Modelo = d.Opciones["printer-make-and-model"]
	d.Ubicacion = d.Opciones["printer-location"]

	c, err := LeerCola(ctx, cola)
	if err != nil || !c.Existe {
		d.Estado = "sin_cola"
		return d
	}
	d.Habilitada, d.Aceptando, d.Razon = c.Habilitada, c.Aceptando, razonLegible(c.Razon)

	usb := impresoraConectada(ctx, cola)
	for _, r := range leerRazones(ctx, cola) {
		if t, ok := motivosError[r]; ok {
			d.Alertas = append(d.Alertas, Alerta{"error", r, t})
		} else if t, ok := motivosAviso[r]; ok {
			d.Alertas = append(d.Alertas, Alerta{"aviso", r, t})
		}
	}
	if !usb {
		d.Alertas = append(d.Alertas, Alerta{"aviso", "P-6", "Impresora desconectada o apagada"})
	}
	if !d.Habilitada && usb {
		d.Alertas = append(d.Alertas, Alerta{"aviso", "P-4", "La cola está detenida" + razon(c.Razon)})
	}
	if !d.Aceptando {
		d.Alertas = append(d.Alertas, Alerta{"aviso", "P-5", "La cola no acepta trabajos"})
	}

	switch {
	case !usb:
		d.Estado = "desconectada"
	case !d.Habilitada || !d.Aceptando:
		d.Estado = "detenida"
	case tieneAlerta(d.Alertas, "error"):
		d.Estado = "error"
	default:
		d.Estado = "lista"
	}
	return d
}

var reOpcion = regexp.MustCompile(`([A-Za-z0-9_.-]+)=('([^']*)'|"([^"]*)"|([^ \n]+))`)

// leerOpciones convierte la línea de `lpoptions -p` en un mapa. Los valores con
// espacios vienen entre comillas; el resto son palabras sueltas.
func leerOpciones(ctx context.Context, cola string) map[string]string {
	m := map[string]string{}
	out, _, err := ejecutar(ctx, 2*time.Second, "lpoptions", "-p", cola)
	if err != nil {
		return m
	}
	for _, g := range reOpcion.FindAllStringSubmatch(out, -1) {
		v := g[3]
		if v == "" {
			v = g[4]
		}
		if v == "" {
			v = g[5]
		}
		m[g[1]] = v
	}
	return m
}

func razonLegible(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "unplugged or turned off":
		return "desconectada o apagada"
	case "paused":
		return "en pausa"
	}
	return strings.TrimSpace(r)
}

// ImprimirPrueba envía la página de prueba estándar de CUPS a la cola. Si la
// impresora está desconectada, CUPS la deja en cola; el panel avisa antes.
func ImprimirPrueba(ctx context.Context, cola string) (string, error) {
	if err := validarCola(ctx, cola); err != nil {
		return "", err
	}
	if _, err := os.Stat(paginaPrueba); err != nil {
		return "", &ErrAccion{Mensaje{"No encuentro la página de prueba de CUPS.", "ls " + paginaPrueba}}
	}
	id, err := Enviar(ctx, []string{"-d", cola, paginaPrueba})
	if err != nil {
		if e, ok := err.(*ErrLP); ok {
			m := TraducirErrorLP(e.Stderr)
			if m.Que == "lp devolvió un error inesperado." {
				m.Que = "No se pudo enviar la prueba: " + strings.TrimSpace(e.Stderr)
			}
			return "", &ErrAccion{m}
		}
		return "", err
	}
	return id, nil
}

func textoDetalles(w io.Writer, d Detalles) {
	fmt.Fprintf(w, "Impresora %s (%s)\n", d.Nombre, d.Cola)
	if !d.Cups {
		fmt.Fprintln(w, "  CUPS no responde.")
		return
	}
	fmt.Fprintf(w, "  URI: %s\n", valorODefecto(d.URI))
	fmt.Fprintf(w, "  Modelo: %s\n", valorODefecto(d.Modelo))
	fmt.Fprintf(w, "  Ubicación: %s\n", valorODefecto(d.Ubicacion))
	est := d.Estado
	if d.Razon != "" && (d.Estado == "detenida" || d.Estado == "error") {
		est += " (" + d.Razon + ")"
	}
	fmt.Fprintf(w, "  Estado: %s\n", est)
	fmt.Fprintf(w, "  Predeterminada: %s\n", map[bool]string{true: "sí", false: "no"}[d.Predeterminada])
	var op []string
	for _, k := range []string{"PageSize", "ColorModel", "Duplex", "print-quality"} {
		if v := d.Opciones[k]; v != "" {
			op = append(op, k+"="+v)
		}
	}
	if len(op) > 0 {
		fmt.Fprintf(w, "  Opciones: %s\n", strings.Join(op, " · "))
	}
}

func valorODefecto(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
