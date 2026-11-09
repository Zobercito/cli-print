package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Sesion struct {
	ID       string            `json:"id"`
	Archivo  string            `json:"archivo"`
	Mtime    int64             `json:"mtime"`
	PDF      string            `json:"pdf"`
	Paginas  int               `json:"paginas"`
	A        []int             `json:"A"`
	R        []int             `json:"R"`
	Estado   string            `json:"estado"`
	Job1     string            `json:"job_pasada1"`
	Copia    int               `json:"copia"`
	Copias   int               `json:"copias"`
	Opciones map[string]string `json:"opciones"`
	Guardada time.Time         `json:"guardada"`
}

func rutaSesion() string { return filepath.Join(dirEstado(), "sesion.json") }

// Guardar escribe de forma atómica (.tmp + rename), spec 9.1.
func (s *Sesion) Guardar() error {
	s.Guardada = time.Now()
	ruta := rutaSesion()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	datos, _ := json.MarshalIndent(s, "", "  ")
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ruta)
}

// CargarSesion devuelve la sesión pendiente válida, o nil. Descarta las de más
// de 24 h o con el archivo modificado (spec 9.1).
func CargarSesion() *Sesion {
	datos, err := os.ReadFile(rutaSesion())
	if err != nil {
		return nil
	}
	var s Sesion
	if json.Unmarshal(datos, &s) != nil || s.Estado == "" || s.Estado == "TERMINADO" {
		return nil
	}
	if time.Since(s.Guardada) > 24*time.Hour {
		BorrarSesion()
		return nil
	}
	if st, err := os.Stat(s.Archivo); err != nil || st.ModTime().Unix() != s.Mtime {
		BorrarSesion()
		return nil
	}
	return &s
}

func BorrarSesion() { os.Remove(rutaSesion()) }
