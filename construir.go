package main

import "strconv"

type Opciones struct {
	Cola        string
	Papel       string // PageSize, ej. "A4"
	Tinta       string // color | normal | gris
	Calidad     string // borrador | normal | alta (vacío = el de la cola)
	Orientacion string // Portrait | Landscape | UpsideDown | Seascape
	Copias      int
	Paginas     []int    // vacío = todas
	Medio       string   // MediaType (vacío = el de la cola)
	SinBordes   bool     // StpFullBleed=True
	Extra       []string // clave=valor ya validadas
	Escala      string   // print-scaling: fit | fill (vacío = ninguna)
	Orden       string   // outputorder: reverse | normal (vacío = el de la cola)
}

var colorModel = map[string]string{"color": "CMY", "normal": "RGB", "gris": "Gray"}
var calidadModel = map[string]string{"borrador": "3", "normal": "4", "alta": "5"}

// ConstruirLP devuelve los argumentos de lp (sin shell). Orden estable.
func ConstruirLP(o Opciones, archivo string) []string {
	args := []string{"-d", o.Cola}
	if o.Copias > 1 {
		args = append(args, "-n", strconv.Itoa(o.Copias))
	}
	if o.Papel != "" {
		args = append(args, "-o", "PageSize="+o.Papel)
	}
	if cm, ok := colorModel[o.Tinta]; ok {
		args = append(args, "-o", "ColorModel="+cm)
	}
	if q, ok := calidadModel[o.Calidad]; ok {
		args = append(args, "-o", "print-quality="+q)
	}
	if o.Orientacion != "" {
		args = append(args, "-o", "StpOrientation="+o.Orientacion)
	}
	if o.Medio != "" {
		args = append(args, "-o", "MediaType="+o.Medio)
	}
	if o.Escala != "" {
		args = append(args, "-o", "print-scaling="+o.Escala)
	}
	if o.SinBordes {
		args = append(args, "-o", "StpFullBleed=True")
	}
	if o.Orden != "" {
		args = append(args, "-o", "outputorder="+o.Orden)
	}
	for _, e := range o.Extra {
		args = append(args, "-o", e)
	}
	if len(o.Paginas) > 0 {
		args = append(args, "-o", "page-ranges="+FormatoRangos(o.Paginas))
	}
	return append(args, "--", archivo)
}
