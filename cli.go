package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"
)

// Resultado es lo que imprimen las acciones con --json.
type Resultado struct {
	OK       bool      `json:"ok"`
	Mensaje  string    `json:"mensaje"`
	Accion   string    `json:"accion,omitempty"`
	Estado   *Estado   `json:"estado,omitempty"`
	Detalles *Detalles `json:"detalles,omitempty"`
}

var subcomandos = map[string]bool{"estado": true, "cancelar": true, "retener": true,
	"liberar": true, "habilitar": true, "imprimir": true, "trabajos": true, "nombres": true,
	"prueba": true, "detalles": true}

// subcomando ejecuta print estado|cancelar|retener|liberar|habilitar|imprimir.
// Devuelve handled=false si el primer argumento no es un subcomando.
func subcomando(ctx context.Context, args []string, cfg Config, salida, errSalida io.Writer) (bool, int) {
	if len(args) == 0 || !subcomandos[args[0]] {
		return false, 0
	}
	nombre := args[0]
	fs := flag.NewFlagSet("print "+nombre, flag.ContinueOnError)
	fs.SetOutput(errSalida)
	var js, todos bool
	cola := cfg.Impresora
	fs.BoolVar(&js, "json", false, "salida en JSON")
	fs.BoolVar(&todos, "todos", false, "todos los trabajos propios")
	fs.StringVar(&cola, "cola", cfg.Impresora, "nombre de la cola")
	var pos []string
	rest := args[1:]
	for {
		if err := fs.Parse(rest); err != nil {
			return true, 2
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	// Sin cola configurada se permite consultar: el panel muestra «sin_cola»
	// y las impresoras conocidas en vez de fallar.
	if nombre != "nombres" && cola != "" && !reColaValida.MatchString(cola) {
		fmt.Fprintln(errSalida, "print: nombre de cola no válido")
		return true, 2
	}
	// Límite global según lo que se pida: consultar es rápido; habilitar e imprimir
	// pueden esperar la contraseña del usuario.
	limite := 20 * time.Second
	switch nombre {
	case "estado", "trabajos":
		limite = 8 * time.Second
	case "detalles":
		limite = 10 * time.Second
	case "prueba":
		limite = 30 * time.Second
	case "habilitar", "imprimir":
		limite = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limite)
	defer cancel()

	if nombre == "estado" || nombre == "trabajos" {
		e := LeerEstado(ctx, cola)
		if js {
			json.NewEncoder(salida).Encode(e)
		} else {
			textoEstado(salida, e, nombre == "trabajos")
		}
		return true, 0
	}

	if nombre == "detalles" {
		d := LeerDetalles(ctx, cola)
		if js {
			json.NewEncoder(salida).Encode(Resultado{OK: true, Mensaje: "hecho", Detalles: &d})
		} else {
			textoDetalles(salida, d)
		}
		return true, 0
	}

	if nombre == "prueba" {
		id, err := ImprimirPrueba(ctx, cola)
		// El panel necesita el estado de la cola que tiene arriba, no el de la
		// impresora previa a la que se manda la prueba.
		estadoCola := cfg.Impresora
		if estadoCola == "" {
			estadoCola = cola
		}
		e := LeerEstado(ctx, estadoCola)
		r := Resultado{OK: err == nil, Estado: &e, Mensaje: "hecho"}
		if err != nil {
			r.Mensaje = err.Error()
			if a, ok := err.(*ErrAccion); ok {
				r.Accion = a.Accion
			}
		} else if id != "" {
			r.Mensaje = "Prueba enviada (" + id + ")"
		}
		if js {
			json.NewEncoder(salida).Encode(r)
		} else if r.OK {
			fmt.Fprintln(salida, r.Mensaje)
			textoEstado(salida, e, true)
		} else {
			fmt.Fprintln(errSalida, "print:", r.Mensaje)
			if r.Accion != "" {
				fmt.Fprintln(errSalida, "      prueba con:", r.Accion)
			}
		}
		if r.OK {
			return true, 0
		}
		return true, 1
	}

	needID := nombre == "cancelar" || nombre == "retener" || nombre == "liberar" || nombre == "imprimir"
	var id string
	if needID && !todos {
		if len(pos) != 1 {
			fmt.Fprintf(errSalida, "uso: print %s <id>%s\n", nombre, map[bool]string{true: " | --todos", false: ""}[nombre == "cancelar" || nombre == "imprimir"])
			return true, 2
		}
		id = pos[0]
	}
	if todos && nombre != "cancelar" && nombre != "imprimir" {
		fmt.Fprintf(errSalida, "print %s no admite --todos\n", nombre)
		return true, 2
	}

	var errs []error
	switch nombre {
	case "nombres":
		if len(pos) != 1 {
			fmt.Fprintln(errSalida, "uso: print nombres '<json>'")
			return true, 2
		}
		if err := GuardarNombresJSON(pos[0]); err != nil {
			errs = []error{err}
		}
	case "cancelar":
		if todos {
			errs = CancelarTodos(ctx, cola)
		} else if err := Cancelar(ctx, id); err != nil {
			errs = []error{err}
		}
	case "retener":
		if err := Retener(ctx, id); err != nil {
			errs = []error{err}
		}
	case "liberar":
		if err := Liberar(ctx, id); err != nil {
			errs = []error{err}
		}
	case "habilitar":
		if err := Habilitar(ctx, cola); err != nil {
			errs = []error{err}
		}
	case "imprimir":
		if todos {
			errs = ImprimirTodos(ctx, cola)
		} else {
			errs = ImprimirUno(ctx, cola, id)
		}
	}
	e := LeerEstado(ctx, cola)
	r := Resultado{OK: len(errs) == 0, Estado: &e, Mensaje: "hecho"}
	if !r.OK {
		r.Mensaje, r.Accion = resumenErrores(errs), accionDe(errs)
	}
	if js {
		json.NewEncoder(salida).Encode(r)
	} else if r.OK {
		fmt.Fprintln(salida, "Hecho.")
		textoEstado(salida, e, true)
	} else {
		fmt.Fprintln(errSalida, "print:", r.Mensaje)
		if r.Accion != "" {
			fmt.Fprintln(errSalida, "      prueba con:", r.Accion)
		}
	}
	if r.OK {
		return true, 0
	}
	return true, 1
}

func textoEstado(w io.Writer, e Estado, soloTrabajos bool) {
	if !soloTrabajos {
		fmt.Fprintf(w, "Impresora %s: %s\n", e.Cola, e.Estado)
		for _, a := range e.Alertas {
			fmt.Fprintf(w, "  %s %s\n", map[string]string{"error": "✗", "aviso": "⚠"}[a.Nivel], a.Texto)
		}
	}
	if len(e.Trabajos) == 0 {
		fmt.Fprintln(w, "Sin trabajos en cola.")
		return
	}
	fmt.Fprintf(w, "%s en cola:\n", pluralTrabajos(len(e.Trabajos)))
	for _, t := range e.Trabajos {
		fmt.Fprintf(w, "  %-18s %-11s %-9s %s\n", t.ID, t.Estado, edadHumana(t.EdadS), t.NombreCorto())
	}
}

// edadHumana: "hace 5 min", "hace 3 h", "ayer" (o "" si se desconoce).
func edadHumana(s int64) string {
	switch {
	case s < 0:
		return "—"
	case s < 60:
		return "ahora"
	case s < 3600:
		return fmt.Sprintf("hace %d min", s/60)
	case s < 86400:
		return fmt.Sprintf("hace %d h", s/3600)
	case s < 172800:
		return "ayer"
	}
	return fmt.Sprintf("hace %d días", s/86400)
}
