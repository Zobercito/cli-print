package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Ajustar: cabe entero sin deformar (quedan franjas). Tamaño real: 1:1, corta lo que
// no cabe. Estirar: llena toda la hoja sin cortar, deformando un poco. Cubrir:
// llena toda la hoja sin deformar, recortando lo que sobra.
// versionEscala va en el nombre de los PDF preparados: si cambia la forma de
// prepararlos, los archivos antiguos de la caché dejan de reutilizarse (antes un
// «real_Letter.pdf» viejo, sin girar, se seguía mandando a la impresora).
const versionEscala = "v3"

var escalas = []par{{"fit", "Ajustar"}, {"real", "Tamaño real"}, {"estirar", "Estirar"}, {"fill", "Cubrir"}}

// Tamaños en puntos de los PageSize habituales.
var papelPts = map[string][2]float64{
	"Letter": {612, 792}, "A4": {595, 842}, "Legal": {612, 1008}, "Executive": {522, 756},
	"Statement": {396, 612}, "Postcard": {283, 420}, "w288h432": {288, 432},
	"w144h432": {144, 432}, "w288h432J": {283, 420},
}

// hayDesajuste: el PDF no tiene el tamaño del papel elegido.
func (m *model) hayDesajuste() bool {
	return m.info.Ancho > 0 && (m.info.Papel != m.papel || m.apaisado())
}

// apaisado: el PDF es horizontal y el papel va en vertical, hay que rotarlo.
func (m *model) apaisado() bool { return m.info.Ancho > m.info.Alto+1 }

func (m *model) visible(f int) bool { return f != rEscala || m.hayDesajuste() }

// escalaArg es el valor de print-scaling que se pasa a lp ("" = ninguno).
func (m *model) escalaArg() string {
	if m.escalaForzada != "" {
		return m.escalaForzada
	}
	if !m.hayDesajuste() {
		return ""
	}
	switch escalas[m.iEscala].Val {
	case "fit", "fill":
		return escalas[m.iEscala].Val
	}
	return ""
}

func (m *model) archivoImp() string {
	if m.pdfImp != "" {
		return m.pdfImp
	}
	return m.pdf
}

// PrepararReal deja el PDF en el papel elegido a 1:1, con el borde superior
// alineado y centrado en horizontal; lo que no cabe se corta por abajo.
func PrepararReal(ctx context.Context, pdf string, ancho, alto float64, papel string) (string, error) {
	dim, ok := papelPts[papel]
	if !ok {
		return "", fmt.Errorf("no conozco el tamaño de %s", papel)
	}
	if ancho <= 0 || alto <= 0 {
		return "", fmt.Errorf("no sé el tamaño del documento")
	}
	clave, err := claveCache(pdf)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dirCache(), clave)
	os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, "real_"+papel+"_"+versionEscala+".pdf")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	if !existe("gs") {
		return "", fmt.Errorf("falta Ghostscript (gs); elige otra escala")
	}
	// Vertical: sin escalar, borde de arriba pegado y centrado; se corta por abajo.
	// Apaisado: se gira 90° en sentido horario (igual que hace CUPS al ajustar) y
	// luego igual: 1:1, arriba pegado y centrado, cortando por abajo.
	var antes string
	if ancho > alto+1 {
		antes = fmt.Sprintf("%.2f %.2f translate -90 rotate", (dim[0]-alto)/2, dim[1])
	} else {
		antes = fmt.Sprintf("%.2f %.2f translate", (dim[0]-ancho)/2, dim[1]-alto)
	}
	_, errs, err := ejecutar(ctx, 60*time.Second, "gs", "-q", "-o", out, "-sDEVICE=pdfwrite",
		fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.0f", dim[0]), fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.0f", dim[1]),
		"-dFIXEDMEDIA", "-dAutoRotatePages=/None", "-c", "<</BeginPage{"+antes+"}>> setpagedevice", "-f", pdf)
	if err != nil {
		return "", fmt.Errorf("ghostscript falló: %v %s", err, errs)
	}
	return out, nil
}

// PrepararEstirar deja el PDF del tamaño exacto del papel escalando el ancho y el
// alto por separado: no sobran franjas ni se corta nada, a costa de deformar un
// poco (con A4 en Carta, ~3 % más ancho y ~6 % más bajo). Un PDF apaisado se gira
// antes para ocupar la hoja vertical.
func PrepararEstirar(ctx context.Context, pdf string, ancho, alto float64, papel string) (string, error) {
	dim, ok := papelPts[papel]
	if !ok {
		return "", fmt.Errorf("no conozco el tamaño de %s", papel)
	}
	if ancho <= 0 || alto <= 0 {
		return "", fmt.Errorf("no sé el tamaño del documento")
	}
	clave, err := claveCache(pdf)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dirCache(), clave)
	os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, "estirar_"+papel+"_"+versionEscala+".pdf")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	if !existe("gs") {
		return "", fmt.Errorf("falta Ghostscript (gs); elige otra escala")
	}
	var antes string
	if ancho > alto+1 { // apaisado: se gira 90° en sentido horario (como CUPS) y luego se escala
		antes = fmt.Sprintf("0 %.0f translate -90 rotate %.6f %.6f scale", dim[1], dim[1]/ancho, dim[0]/alto)
	} else {
		antes = fmt.Sprintf("%.6f %.6f scale", dim[0]/ancho, dim[1]/alto)
	}
	_, errs, err := ejecutar(ctx, 60*time.Second, "gs", "-q", "-o", out, "-sDEVICE=pdfwrite",
		fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.0f", dim[0]), fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.0f", dim[1]),
		"-dFIXEDMEDIA", "-dAutoRotatePages=/None", "-c", "<</BeginPage{"+antes+"}>> setpagedevice", "-f", pdf)
	if err != nil {
		return "", fmt.Errorf("ghostscript falló: %v %s", err, errs)
	}
	return out, nil
}

// Como el papel: solo se ven dos opciones y, a la derecha, «Más». La que
// se elige ahí ocupa el tercer lugar.
func (m *model) escalasVisibles() []int {
	if len(m.escalaVis) == 0 {
		return []int{0, 1}
	}
	return m.escalaVis
}

func posEn(l []int, v int) int {
	for i, x := range l {
		if x == v {
			return i
		}
	}
	return -1
}

func (m *model) fijarEscala(i int) {
	vis := m.escalasVisibles()
	m.iEscala = i
	pos := posEn(vis, i)
	if pos < 0 {
		vis = []int{vis[0], vis[1], i}
		pos = 2
	}
	m.escalaVis, m.escalaCur = vis, pos
}
