package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

var extOK = map[string]bool{
	".pdf": true, ".docx": true, ".doc": true, ".odt": true, ".ods": true, ".odp": true,
	".pptx": true, ".ppt": true, ".xlsx": true, ".xls": true, ".rtf": true, ".txt": true,
	".md": true, ".markdown": true, ".html": true, ".htm": true, ".csv": true,
}

var extImagen = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true,
	".webp": true, ".svg": true, ".tiff": true, ".heic": true,
}

func EsDocumento(ruta string) bool { return extOK[strings.ToLower(filepath.Ext(ruta))] }
func EsImagen(ruta string) bool    { return extImagen[strings.ToLower(filepath.Ext(ruta))] }

type Archivo struct {
	Ruta   string
	Nombre string
	Mtime  int64
}

// ListarDocumentos recorre dirs hasta prof niveles, sin carpetas ocultas.
func ListarDocumentos(dirs []string, prof int) []Archivo {
	var out []Archivo
	vistos := map[string]bool{}
	for _, raiz := range dirs {
		raiz = filepath.Clean(raiz)
		filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(raiz, p)
				nivel := 0
				if rel != "." {
					nivel = strings.Count(rel, string(filepath.Separator)) + 1
				}
				if nivel > prof || (p != raiz && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !EsDocumento(p) || vistos[p] {
				return nil
			}
			if i, e := d.Info(); e == nil && i.Mode().IsRegular() {
				vistos[p] = true
				out = append(out, Archivo{p, d.Name(), i.ModTime().Unix()})
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })
	return out
}

// Coincidencia es un resultado de búsqueda difusa con las posiciones a resaltar.
type Coincidencia struct {
	Archivo
	Puntos int
	Pos    []int // índices de rune en Nombre
}

// Buscar: subsecuencia sin distinguir mayúsculas ni tildes. Premia coincidencias
// consecutivas, inicio de palabra y nombres cortos. q vacío devuelve todo.
func Buscar(lista []Archivo, q string) []Coincidencia {
	qs := []rune(strings.ReplaceAll(plano(q), " ", ""))
	var out []Coincidencia
	for _, a := range lista {
		if len(qs) == 0 {
			out = append(out, Coincidencia{Archivo: a})
			continue
		}
		nom := []rune(a.Nombre)
		pl := []rune(plano(a.Nombre))
		var pos []int
		pts, j, prev := 0, 0, -2
		for i := 0; i < len(pl) && j < len(qs); i++ {
			if pl[i] == qs[j] {
				pts += 1
				if i == prev+1 {
					pts += 3
				}
				if i == 0 || !unicode.IsLetter(pl[i-1]) && !unicode.IsDigit(pl[i-1]) {
					pts += 2
				}
				pos = append(pos, i)
				prev = i
				j++
			}
		}
		if j == len(qs) {
			out = append(out, Coincidencia{a, pts*10 - len(nom)/4, pos})
		}
	}
	if len(qs) > 0 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Puntos > out[j].Puntos })
	}
	return out
}

func plano(s string) string {
	r := []rune(strings.ToLower(s))
	for i, c := range r {
		switch c {
		case 'á', 'à', 'ä':
			r[i] = 'a'
		case 'é', 'è', 'ë':
			r[i] = 'e'
		case 'í', 'ï':
			r[i] = 'i'
		case 'ó', 'ö':
			r[i] = 'o'
		case 'ú', 'ü':
			r[i] = 'u'
		case 'ñ':
			r[i] = 'n'
		}
	}
	return string(r)
}

// Validar comprueba una ruta dada por el usuario (F-01..F-04).
func Validar(ruta string) *Mensaje {
	st, err := os.Stat(ruta)
	switch {
	case os.IsNotExist(err):
		return &Mensaje{"No existe: " + ruta, "Revisa la ruta o ejecuta print sin argumentos para buscarla"}
	case os.IsPermission(err):
		return &Mensaje{"No tengo permiso para leer " + ruta, "Revisa los permisos con ls -l"}
	case err != nil:
		return &Mensaje{err.Error(), ""}
	case st.IsDir():
		return nil
	case EsImagen(ruta):
		return &Mensaje{"Solo se imprimen documentos; " + strings.ToLower(filepath.Ext(ruta)) + " no está soportado.", ""}
	case !EsDocumento(ruta):
		return &Mensaje{"Formato no soportado: " + filepath.Ext(ruta), "Soportados: pdf docx doc odt ods odp pptx ppt xlsx xls rtf txt md html csv"}
	case st.Size() == 0:
		return &Mensaje{"El archivo está vacío.", ""}
	}
	if f, err := os.Open(ruta); err != nil {
		return &Mensaje{"No tengo permiso para leer " + ruta, "Revisa los permisos con ls -l"}
	} else {
		f.Close()
	}
	return nil
}
