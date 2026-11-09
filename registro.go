package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// dirEstado es ~/.local/state/print (o PRINT_ESTADO en las pruebas).
func dirEstado() string {
	if d := os.Getenv("PRINT_ESTADO"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "state", "print")
}

// Envio recuerda qué archivo corresponde a un trabajo que mandó este programa:
// CUPS no guarda la ruta, solo el título, y así se distingue «origen: print».
type Envio struct {
	Archivo string    `json:"archivo"`
	Hora    time.Time `json:"hora"`
}

func rutaRegistro() string { return filepath.Join(dirEstado(), "enviados.json") }

func LeerRegistro() map[string]Envio {
	out := map[string]Envio{}
	if datos, err := os.ReadFile(rutaRegistro()); err == nil {
		json.Unmarshal(datos, &out)
	}
	return out
}

// RegistrarEnvio guarda el trabajo (escritura atómica, máximo 200 entradas).
func RegistrarEnvio(id, archivo string) {
	reg := LeerRegistro()
	reg[id] = Envio{Archivo: archivo, Hora: time.Now()}
	if len(reg) > 200 {
		type par struct {
			id string
			t  time.Time
		}
		var l []par
		for k, v := range reg {
			l = append(l, par{k, v.Hora})
		}
		sort.Slice(l, func(i, j int) bool { return l[i].t.Before(l[j].t) })
		for _, p := range l[:len(l)-200] {
			delete(reg, p.id)
		}
	}
	if os.MkdirAll(dirEstado(), 0o755) != nil {
		return
	}
	datos, _ := json.Marshal(reg)
	tmp := rutaRegistro() + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) == nil {
		os.Rename(tmp, rutaRegistro())
	}
}
