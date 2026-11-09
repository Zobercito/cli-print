package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type EstadoCola struct {
	Existe, Habilitada, Aceptando bool
	Razon                         string // ej. "Unplugged or turned off"
}

// LeerCola consulta lpstat -p y -a (salida en C, ver ejecutar).
func LeerCola(ctx context.Context, cola string) (EstadoCola, error) {
	var e EstadoCola
	out, errs, err := ejecutar(ctx, 3*time.Second, "lpstat", "-p", cola)
	if err == ErrTiempo {
		return e, err
	}
	if err != nil || strings.Contains(errs, "Invalid destination") {
		return e, nil // no existe
	}
	e.Existe = true
	e.Habilitada, e.Razon = parseP(out, cola)
	a, _, _ := ejecutar(ctx, 3*time.Second, "lpstat", "-a", cola)
	e.Aceptando = strings.Contains(a, "accepting requests") && !strings.Contains(a, "not accepting")
	return e, nil
}

func parseP(out, cola string) (bool, string) {
	lineas := strings.Split(strings.TrimSpace(out), "\n")
	if len(lineas) == 0 {
		return false, ""
	}
	if strings.Contains(lineas[0], " disabled") {
		razon := ""
		if len(lineas) > 1 {
			razon = strings.TrimSpace(lineas[1])
		}
		return false, razon
	}
	return true, ""
}

// ColaPorDefecto devuelve la cola predeterminada del sistema ("" si no hay).
func ColaPorDefecto() string {
	out, _, err := ejecutar(context.Background(), 3*time.Second, "lpstat", "-d")
	if err != nil {
		return ""
	}
	if i := strings.Index(out, ": "); i >= 0 {
		return strings.TrimSpace(out[i+2:])
	}
	return ""
}

var reID = regexp.MustCompile(`request id is (\S+)`)

// Enviar ejecuta lp con argumentos separados (nunca shell) y devuelve el ID.
func Enviar(ctx context.Context, args []string) (string, error) {
	out, errs, err := ejecutar(ctx, 20*time.Second, "lp", args...)
	if err != nil {
		return "", &ErrLP{Err: err, Stderr: strings.TrimSpace(errs + out)}
	}
	m := reID.FindStringSubmatch(out)
	if m == nil {
		return "", &ErrLP{Err: fmt.Errorf("lp no devolvió un ID de trabajo"), Stderr: out}
	}
	return m[1], nil
}

type ErrLP struct {
	Err    error
	Stderr string
}

func (e *ErrLP) Error() string { return e.Err.Error() + ": " + e.Stderr }

type EstadoTrabajo int

const (
	TrabajoActivo EstadoTrabajo = iota
	TrabajoTerminado
	TrabajoDesconocido
)

// ConsultarTrabajo mira solo el ID dado (13.2).
func ConsultarTrabajo(ctx context.Context, cola, id string) EstadoTrabajo {
	out, _, err := ejecutar(ctx, 3*time.Second, "lpstat", "-W", "not-completed", "-o", cola)
	if err != nil && err != ErrTiempo {
		return TrabajoDesconocido
	}
	if enLista(out, id) {
		return TrabajoActivo
	}
	out, _, _ = ejecutar(ctx, 3*time.Second, "lpstat", "-W", "completed", "-o", cola)
	if enLista(out, id) {
		return TrabajoTerminado
	}
	return TrabajoDesconocido
}

func enLista(out, id string) bool {
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == id {
			return true
		}
	}
	return false
}
