package main

import (
	"context"
	"errors"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type InfoPDF struct {
	Paginas  int    // 0 = desconocido (F-05)
	Papel    string // nombre PageSize del PPD, "" si no se reconoce
	Cifrado  bool
	Ancho    float64
	Alto     float64
	Mezclado bool
}

var (
	rePaginas = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)
	reTam     = regexp.MustCompile(`(?m)^Page size:\s+([\d.]+) x ([\d.]+) pts`)
)

var ErrPDFCifrado = errors.New("el PDF está protegido con contraseña")
var ErrPDFSinPermiso = errors.New("el PDF no permite imprimir")
var ErrPDFDanado = errors.New("el PDF está dañado o incompleto")

var reSinImprimir = regexp.MustCompile(`Encrypted:\s+yes \([^)]*print:no`)

// AnalizarPDF: pdfinfo -> qpdf -> regex (F-05).
func AnalizarPDF(ctx context.Context, ruta string) (InfoPDF, error) {
	var info InfoPDF
	if existe("pdfinfo") {
		out, errs, err := ejecutar(ctx, 10*time.Second, "pdfinfo", ruta)
		if strings.Contains(errs, "Incorrect password") || strings.Contains(out, "Encrypted:      yes") && err != nil {
			return info, ErrPDFCifrado
		}
		if err == nil {
			if reSinImprimir.MatchString(out) {
				return info, ErrPDFSinPermiso
			}
			if m := rePaginas.FindStringSubmatch(out); m != nil {
				info.Paginas, _ = strconv.Atoi(m[1])
			}
			if m := reTam.FindStringSubmatch(out); m != nil {
				info.Ancho, _ = strconv.ParseFloat(m[1], 64)
				info.Alto, _ = strconv.ParseFloat(m[2], 64)
				info.Papel = NombrePapel(info.Ancho, info.Alto)
			}
			return info, nil
		}
	}
	// pdfinfo no pudo leerlo: si qpdf tampoco lo da por válido, está dañado
	// (un PDF cortado antes pasaba y CUPS fallaba o imprimía basura).
	if existe("pdfinfo") && existe("qpdf") {
		if _, errs, err := ejecutar(ctx, 20*time.Second, "qpdf", "--check", ruta); err != nil && !strings.Contains(errs, "password") {
			return info, ErrPDFDanado
		}
	}
	if existe("qpdf") {
		out, errs, err := ejecutar(ctx, 10*time.Second, "qpdf", "--show-npages", ruta)
		if strings.Contains(errs, "password") {
			return info, ErrPDFCifrado
		}
		if n, e := strconv.Atoi(strings.TrimSpace(out)); err == nil && e == nil {
			info.Paginas = n
			return info, nil
		}
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return info, err
	}
	if !strings.HasPrefix(string(datos[:min(len(datos), 8)]), "%PDF") {
		return info, errors.New("no parece un PDF válido")
	}
	info.Paginas = len(regexp.MustCompile(`/Type\s*/Page[^s]`).FindAll(datos, -1))
	return info, nil
}

type papelDef struct {
	nombre string
	w, h   float64
}

var papeles = []papelDef{
	{"Letter", 612, 792}, {"A4", 595, 842}, {"Legal", 612, 1008},
	{"A5", 420, 595}, {"A6", 298, 420}, {"B5", 499, 709},
}

// NombrePapel devuelve el PageSize más cercano (tolerancia 4 pt) o "".
func NombrePapel(w, h float64) string {
	if w > h {
		w, h = h, w
	}
	for _, p := range papeles {
		if math.Abs(p.w-w) <= 4 && math.Abs(p.h-h) <= 4 {
			return p.nombre
		}
	}
	return ""
}
