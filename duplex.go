package main

// PlanDuplex describe las dos pasadas del doble cara manual (spec §8.2).
type PlanDuplex struct {
	A          []int // frentes de cada hoja, ascendente
	R          []int // reversos de cada hoja, ascendente
	Hojas      int
	SinReverso int // página que queda sin reverso (0 si ninguna)

	// Orden real de impresión. La impresora saca las hojas boca arriba y la última
	// queda encima. Para que al terminar la página 1 quede arriba (con la 2 al
	// voltearla) hay que imprimir primero los reversos y al final los frentes,
	// cada tanda de la última hoja a la primera.
	Pasada1 []int // reversos, de mayor a menor
	Pasada2 []int // frentes, de mayor a menor
}

// PlanificarDuplex agrupa por posición en la selección, no por paridad.
func PlanificarDuplex(sel []int) PlanDuplex {
	var p PlanDuplex
	for i, pag := range sel {
		if i%2 == 0 {
			p.A = append(p.A, pag)
		} else {
			p.R = append(p.R, pag)
		}
	}
	p.Pasada1, p.Pasada2 = invertida(p.R), invertida(p.A)
	p.Hojas = len(p.A)
	if len(sel)%2 == 1 {
		p.SinReverso = sel[len(sel)-1]
	}
	return p
}

// OrientacionReverso devuelve la orientación girada 180° (spec §8.4).
func OrientacionReverso(o string) string {
	switch o {
	case "Portrait":
		return "UpsideDown"
	case "Landscape":
		return "Seascape"
	case "UpsideDown":
		return "Portrait"
	case "Seascape":
		return "Landscape"
	}
	return o
}

// DuplexUtil indica si el doble cara tiene sentido (>= 2 páginas).
func DuplexUtil(sel []int) bool { return len(sel) >= 2 }

func invertida(l []int) []int {
	out := make([]int, len(l))
	for i, v := range l {
		out[len(l)-1-i] = v
	}
	return out
}
