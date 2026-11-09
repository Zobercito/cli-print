package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PrepararPasadas crea los dos PDF del doble cara con las páginas YA en el orden
// en que deben imprimirse. CUPS solo sabe imprimir rangos en orden ascendente,
// así que el orden inverso se resuelve reordenando el PDF con qpdf.
//
// Con un total impar la última hoja no tiene reverso, pero tiene que estar en
// el bloque desde la pasada 1 para quedar en su sitio. Se añade una página en
// blanco al principio de la pasada 1 (la impresora pasa una hoja sin tinta) y
// así el usuario no tiene que poner nada a mano. enBlanco indica si se pudo;
// si no (falta gs) el usuario debe añadir la hoja él.
func PrepararPasadas(ctx context.Context, pdf string, pl PlanDuplex) (p1, p2 string, enBlanco bool, err error) {
	if !existe("qpdf") {
		return "", "", false, fmt.Errorf("qpdf no está instalado")
	}
	clave, err := claveCache(pdf)
	if err != nil {
		return "", "", false, err
	}
	dir := filepath.Join(dirCache(), clave)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", false, err
	}
	blanco := ""
	if pl.SinReverso > 0 {
		blanco = paginaEnBlanco(ctx, pdf, dir)
	}
	var rutas [2]string
	for i, paginas := range [][]int{pl.Pasada1, pl.Pasada2} {
		if len(paginas) == 0 {
			return "", "", false, fmt.Errorf("la pasada %d no tiene páginas", i+1)
		}
		partes := make([]string, len(paginas))
		for j, p := range paginas {
			partes[j] = strconv.Itoa(p)
		}
		lista := strings.Join(partes, ",")
		args := []string{"--empty", "--pages"}
		conBlanco := i == 0 && blanco != ""
		if conBlanco {
			args = append(args, blanco, "1")
		}
		args = append(args, pdf, lista, "--")
		clavePasada := lista
		if conBlanco {
			clavePasada = "B," + lista
		}
		h := sha256.Sum256([]byte(clavePasada))
		out := filepath.Join(dir, fmt.Sprintf("pasada%d_%s.pdf", i+1, hex.EncodeToString(h[:4])))
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			rutas[i] = out
			continue
		}
		_, errs, err := ejecutar(ctx, 30*time.Second, "qpdf", append(args, out)...)
		// qpdf sale con 3 cuando solo hay avisos; vale si el archivo existe
		if st, e := os.Stat(out); e != nil || st.Size() == 0 {
			return "", "", false, fmt.Errorf("qpdf no pudo reordenar las páginas: %v %s", err, strings.TrimSpace(errs))
		}
		rutas[i] = out
	}
	return rutas[0], rutas[1], blanco != "", nil
}

// paginaEnBlanco crea (o reutiliza) un PDF de una página en blanco del mismo
// tamaño que el documento. Devuelve "" si no se puede.
func paginaEnBlanco(ctx context.Context, pdf, dir string) string {
	if !existe("gs") {
		return ""
	}
	w, h := 612.0, 792.0
	if info, err := AnalizarPDF(ctx, pdf); err == nil && info.Ancho > 0 && info.Alto > 0 {
		w, h = info.Ancho, info.Alto
	}
	out := filepath.Join(dir, fmt.Sprintf("blanco_%.0fx%.0f.pdf", w, h))
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out
	}
	ejecutar(ctx, 20*time.Second, "gs", "-q", "-dNOPAUSE", "-dBATCH", "-sDEVICE=pdfwrite",
		"-sOutputFile="+out, fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.0f", w), fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.0f", h),
		"-c", "showpage")
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return ""
	}
	return out
}
