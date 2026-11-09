package main

import (
	"context"
	"regexp"
	"strings"
	"time"
)

var etiquetasPapel = map[string]string{
	"Letter": "Carta", "w144h432": "2×6", "w288h432": "4×6", "w288h432J": "KG",
}

func etiquetaPapel(n string) string {
	if e, ok := etiquetasPapel[n]; ok {
		return e
	}
	return n
}

// valoresDeCola pide a CUPS las opciones que conoce para una cola
// (lpoptions -l). No depende de un PPD ni de un modelo de impresora.
func valoresDeCola(cola, clave string) []string {
	if cola == "" {
		return nil
	}
	out, _, err := ejecutar(context.Background(), 2*time.Second, "lpoptions", "-p", cola, "-l")
	if err != nil {
		return nil
	}
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, clave+"/") && !strings.HasPrefix(l, clave+":") {
			continue
		}
		i := strings.Index(l, ":")
		if i < 0 {
			continue
		}
		var res []string
		for _, v := range strings.Fields(l[i+1:]) {
			res = append(res, strings.TrimPrefix(v, "*"))
		}
		return res
	}
	return nil
}

// TamanosPapel devuelve los PageSize que ofrece la cola. Carta y A4 van
// primero. Si no se puede consultar, usa una lista habitual.
func TamanosPapel(cola string) []string {
	fijo := []string{"Letter", "A4", "Legal", "Executive", "Statement", "Postcard", "w288h432"}
	out := valoresDeCola(cola, "PageSize")
	if len(out) == 0 {
		return fijo
	}
	var limpio []string
	for _, n := range out {
		if n == "" || strings.HasPrefix(n, "CD") || n == "Manual" || n == "Custom" {
			continue
		}
		limpio = append(limpio, n)
	}
	if len(limpio) == 0 {
		return fijo
	}
	res := []string{}
	for _, n := range []string{"Letter", "A4"} {
		if indiceDe(limpio, n) >= 0 {
			res = append(res, n)
		}
	}
	for _, n := range limpio {
		if n != "Letter" && n != "A4" {
			res = append(res, n)
		}
	}
	return res
}

// papelDeCola devuelve el PageSize predeterminado de la cola ("" si no se sabe).
func papelDeCola(ctx context.Context, cola string) string {
	if cola == "" {
		return ""
	}
	out, _, err := ejecutar(ctx, 2*time.Second, "lpoptions", "-p", cola)
	if err != nil {
		return ""
	}
	for _, f := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(f, "PageSize="); ok {
			return v
		}
	}
	return ""
}

var (
	rePrinterInfo = regexp.MustCompile(`printer-info='([^']*)'`)
	reMakeModel   = regexp.MustCompile(`printer-make-and-model='([^']*)'`)
)

// nombreImpresora devuelve un nombre legible para la cola. Usa la descripción
// que declara CUPS, luego el modelo del controlador y, si no hay nada, el
// nombre de la cola con guiones bajos convertidos en espacios.
func nombreImpresora(ctx context.Context, cola string) string {
	if cola == "" {
		return "Impresora"
	}
	if a := aliasDe(cola); a != "" {
		return a
	}
	out, _, err := ejecutar(ctx, 2*time.Second, "lpoptions", "-p", cola)
	if err == nil {
		for _, re := range []*regexp.Regexp{rePrinterInfo, reMakeModel} {
			if m := re.FindStringSubmatch(out); m != nil {
				if n := strings.TrimSpace(m[1]); n != "" {
					return n
				}
			}
		}
	}
	return strings.ReplaceAll(cola, "_", " ")
}

func indiceDe(l []string, s string) int {
	for i, v := range l {
		if v == s {
			return i
		}
	}
	return -1
}

// fijarPapel elige un tamaño y lo hace visible: Carta, A4 y, en tercer lugar,
// el último elegido entre los demás.
func (m *model) fijarPapel(n string) {
	m.papel = n
	if indiceDe(m.papelVis, n) < 0 {
		m.papelVis = []string{m.papelVis[0], m.papelVis[1], n}
	}
	m.papelCur = indiceDe(m.papelVis, n)
}

// ValoresPPD devuelve las opciones válidas de una clave en la cola.
func ValoresPPD(cola, clave string) []string {
	return valoresDeCola(cola, clave)
}
