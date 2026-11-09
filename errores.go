package main

import "strings"

// Mensaje es un error explicado: qué pasó y qué hacer (principio 1).
type Mensaje struct{ Que, Accion string }

// Texto junta qué pasó y qué hacer, sin espacios sobrantes.
func (m Mensaje) Texto() string {
	if m.Accion == "" {
		return strings.TrimSpace(m.Que)
	}
	return strings.TrimSpace(m.Que) + ". " + strings.TrimSpace(m.Accion)
}

var tablaLP = []struct {
	patron string
	m      Mensaje
}{
	{"does not exist", Mensaje{"La impresora o cola no existe.", "Revisa el nombre con: lpstat -p"}},
	{"Unable to connect", Mensaje{"No hay conexión con CUPS.", "sudo systemctl start cups"}},
	{"Connection refused", Mensaje{"CUPS no está en ejecución.", "sudo systemctl start cups"}},
	{"not accepting", Mensaje{"La cola no acepta trabajos.", "sudo cupsaccept <impresora>"}},
	{"forbidden", Mensaje{"CUPS rechazó la operación por permisos.", "Ejecútala tú con sudo"}},
	{"Unsupported format", Mensaje{"CUPS no entiende el formato del archivo.", "Convierte el archivo a PDF primero"}},
	{"No such file", Mensaje{"El archivo no existe o se movió.", "Recarga con r y elígelo otra vez"}},
	{"Bad file descriptor", Mensaje{"No se pudo leer el archivo.", "Revisa los permisos del archivo"}},
}

// TraducirErrorLP convierte el stderr de lp/lpstat en un mensaje claro.
func TraducirErrorLP(stderr string) Mensaje {
	for _, f := range tablaLP {
		if strings.Contains(stderr, f.patron) {
			return f.m
		}
	}
	return Mensaje{"lp devolvió un error inesperado.", "Mira el texto original de abajo"}
}
