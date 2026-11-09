package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// ErrAccion es un fallo explicado (qué pasó y qué hacer).
type ErrAccion struct{ Mensaje }

func (e *ErrAccion) Error() string { return e.Que }

// errYaNoExiste: el trabajo desapareció entre leer y actuar. No es grave.
var errYaNoExiste = &ErrAccion{Mensaje{"Ese trabajo ya no existe.", ""}}

func validarCola(ctx context.Context, cola string) error {
	if !reColaValida.MatchString(cola) {
		return &ErrAccion{Mensaje{"Nombre de cola no válido: " + cola, ""}}
	}
	c, err := LeerCola(ctx, cola)
	if err != nil || !c.Existe {
		return &ErrAccion{Mensaje{"La cola " + cola + " no existe.", "Revisa con: lpstat -p"}}
	}
	return nil
}

func validarID(id string) error {
	if !reIDTrabajo.MatchString(id) {
		return &ErrAccion{Mensaje{"Identificador de trabajo no válido: " + id, ""}}
	}
	return nil
}

func tradErrorCancel(id, stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "not exist") || strings.Contains(s, "not found") || strings.Contains(s, "no existe"):
		return errYaNoExiste
	case strings.Contains(s, "unauthorized") || strings.Contains(s, "forbidden") || strings.Contains(s, "not authorized") || strings.Contains(s, "permission"):
		return &ErrAccion{Mensaje{"No tienes permiso para esa acción sobre el trabajo " + id + ".", "sudo cancel " + id}}
	}
	return &ErrAccion{Mensaje{"No se pudo completar la acción: " + strings.TrimSpace(stderr), ""}}
}

// Cancelar cancela un trabajo propio.
func Cancelar(ctx context.Context, id string) error {
	if err := validarID(id); err != nil {
		return err
	}
	_, errs, err := ejecutar(ctx, 10*time.Second, "cancel", id)
	if err != nil {
		return tradErrorCancel(id, errs)
	}
	return nil
}

// Retener deja un trabajo en espera (hold) y Liberar lo reanuda (resume).
func Retener(ctx context.Context, id string) error { return cambiarRetencion(ctx, id, "hold") }
func Liberar(ctx context.Context, id string) error { return cambiarRetencion(ctx, id, "resume") }

func cambiarRetencion(ctx context.Context, id, h string) error {
	if err := validarID(id); err != nil {
		return err
	}
	_, errs, err := ejecutar(ctx, 10*time.Second, "lp", "-i", id, "-H", h)
	if err != nil {
		return tradErrorCancel(id, errs)
	}
	return nil
}

// Habilitar activa la cola con permisos de administrador: sudo si hay terminal
// (pide la contraseña ahí), pkexec si no (ventana gráfica). El programa nunca
// ve la contraseña.
func Habilitar(ctx context.Context, cola string) error {
	if err := validarCola(ctx, cola); err != nil {
		return err
	}
	for _, cmd := range []string{"cupsenable", "cupsaccept"} {
		c := ordenAdmin(cmd, cola)
		if cmd == "cupsaccept" {
			if e, _ := LeerCola(ctx, cola); e.Aceptando {
				continue
			}
		} else if e, _ := LeerCola(ctx, cola); e.Habilitada {
			continue
		}
		cc, cancel := context.WithTimeout(ctx, 2*time.Minute) // da tiempo a escribir la contraseña
		x := exec.CommandContext(cc, c[0], c[1:]...)
		x.Stdin, x.Stdout, x.Stderr = os.Stdin, os.Stderr, os.Stderr
		err := x.Run()
		cancel()
		if err != nil {
			return &ErrAccion{Mensaje{"No se pudo habilitar la impresora.", strings.Join(c, " ")}}
		}
	}
	return nil
}

// ordenAdmin devuelve [sudo|pkexec] cmd cola.
func ordenAdmin(cmd, cola string) []string {
	if term.IsTerminal(int(os.Stdin.Fd())) && existe("sudo") {
		return []string{"sudo", cmd, cola}
	}
	if existe("pkexec") {
		return []string{"pkexec", cmd, cola}
	}
	return []string{"sudo", cmd, cola}
}

// ImprimirTodos libera los trabajos retenidos y habilita la cola.
func ImprimirTodos(ctx context.Context, cola string) []error {
	var errs []error
	e := LeerEstado(ctx, cola)
	for _, t := range e.Trabajos {
		if t.Propio && t.Estado == "retenido" {
			if err := Liberar(ctx, t.ID); err != nil && err != errYaNoExiste {
				errs = append(errs, err)
			}
		}
	}
	if !e.Habilitada || !e.Aceptando {
		if err := Habilitar(ctx, cola); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// ImprimirUno retiene los demás trabajos propios, libera este y habilita la
// cola: los otros quedan en espera, no se pierden ni se imprimen.
func ImprimirUno(ctx context.Context, cola, id string) []error {
	if err := validarID(id); err != nil {
		return []error{err}
	}
	e := LeerEstado(ctx, cola)
	var objetivo *Trabajo
	for i := range e.Trabajos {
		if e.Trabajos[i].ID == id {
			objetivo = &e.Trabajos[i]
		}
	}
	if objetivo == nil {
		return []error{errYaNoExiste}
	}
	var errs []error
	for _, t := range e.Trabajos {
		if t.ID != id && t.Propio && t.Estado != "retenido" && t.Estado != "imprimiendo" {
			if err := Retener(ctx, t.ID); err != nil && err != errYaNoExiste {
				errs = append(errs, err)
			}
		}
	}
	if objetivo.Estado == "retenido" {
		if err := Liberar(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	if !e.Habilitada || !e.Aceptando {
		if err := Habilitar(ctx, cola); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// CancelarTodos cancela todos los trabajos propios de la cola.
func CancelarTodos(ctx context.Context, cola string) []error {
	var errs []error
	for _, t := range LeerEstado(ctx, cola).Trabajos {
		if !t.Propio {
			continue
		}
		if err := Cancelar(ctx, t.ID); err != nil && err != errYaNoExiste {
			errs = append(errs, err)
		}
	}
	return errs
}

func resumenErrores(errs []error) string {
	var p []string
	for _, e := range errs {
		p = append(p, e.Error())
	}
	return strings.Join(p, "; ")
}

func accionDe(errs []error) string {
	for _, e := range errs {
		if a, ok := e.(*ErrAccion); ok && a.Accion != "" {
			return a.Accion
		}
	}
	return ""
}
