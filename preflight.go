package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Hallazgo struct {
	ID         string
	Bloqueante bool
	Grave      bool // deshabilita Imprimir pero deja ver el menú
	Mensaje
}

// Preflight corre los chequeos en paralelo con 5 s máximo.
func Preflight(ctx context.Context, cola string) (string, []Hallazgo) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	var hs []Hallazgo
	// `actual` la escribe la goroutine de P-3 y la leen P-6 y el final: todo
	// acceso pasa por el mutex para no tener una carrera de datos.
	actual := cola
	add := func(h Hallazgo) { mu.Lock(); hs = append(hs, h); mu.Unlock() }
	leerCola := func() string { mu.Lock(); defer mu.Unlock(); return actual }
	cambiarCola := func(n string) { mu.Lock(); actual = n; mu.Unlock() }

	if !existe("lp") || !existe("lpstat") {
		return cola, []Hallazgo{{ID: "P-1", Bloqueante: true, Mensaje: Mensaje{"CUPS no está instalado.", "sudo pacman -S cups"}}}
	}
	if _, _, err := ejecutar(ctx, 2*time.Second, "lpstat", "-r"); err != nil {
		return cola, []Hallazgo{{ID: "P-2", Bloqueante: true, Mensaje: Mensaje{"CUPS no responde.", "sudo systemctl start cups"}}}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { // P-3..P-5, P-7
		defer wg.Done()
		c := leerCola()
		e, err := LeerCola(ctx, c)
		if err != nil || !e.Existe {
			out, _, _ := ejecutar(ctx, 2*time.Second, "lpstat", "-d")
			if i := strings.Index(out, ": "); i >= 0 && strings.TrimSpace(out[i+2:]) != "" {
				nueva := strings.TrimSpace(out[i+2:])
				add(Hallazgo{ID: "P-3", Mensaje: Mensaje{"No encuentro " + c + "; uso " + nueva + ".", ""}})
				cambiarCola(nueva)
				return
			}
			add(Hallazgo{ID: "P-3", Bloqueante: true, Mensaje: Mensaje{"No hay ninguna impresora configurada.", "Configúrala en http://localhost:631"}})
			return
		}
		if !e.Habilitada {
			h := Hallazgo{ID: "P-4", Grave: true, Mensaje: Mensaje{"La impresora está deshabilitada" + razon(e.Razon) + ".", "sudo cupsenable " + c}}
			if strings.Contains(strings.ToLower(e.Razon), "usblp") {
				h.ID, h.Accion = "P-7", "El módulo usblp del kernel puede chocar con CUPS; desactívalo para esa impresora"
			}
			add(h)
		}
		if !e.Aceptando {
			add(Hallazgo{ID: "P-5", Grave: true, Mensaje: Mensaje{"La cola no acepta trabajos.", "sudo cupsaccept " + c}})
		}
	}()
	go func() { // P-6
		defer wg.Done()
		if !impresoraConectada(ctx, leerCola()) {
			add(Hallazgo{ID: "P-6", Grave: true, Mensaje: Mensaje{"No detecto la impresora por USB.", "Conéctala y enciéndela, luego pulsa r"}})
		}
	}()
	go func() { // P-8..P-10
		defer wg.Done()
		if !existe("soffice") {
			add(Hallazgo{ID: "P-8", Mensaje: Mensaje{"LibreOffice no está instalado: solo se podrán imprimir PDF.", "sudo pacman -S libreoffice-fresh"}})
		}
		if !existe("pdfinfo") {
			add(Hallazgo{ID: "P-9", Mensaje: Mensaje{"pdfinfo no está disponible; uso métodos alternativos.", "sudo pacman -S poppler"}})
		}
		if existe("fc-match") {
			out, _, _ := ejecutar(ctx, 2*time.Second, "fc-match", "Calibri")
			if !strings.Contains(out, "Carlito") {
				add(Hallazgo{ID: "P-10", Mensaje: Mensaje{"Fuentes de Office ausentes: las páginas pueden cambiar.", "sudo pacman -S ttf-carlito ttf-caladea"}})
			}
		}
	}()
	wg.Wait()
	sort.Slice(hs, func(i, j int) bool { return hs[i].ID < hs[j].ID })
	return leerCola(), hs
}

// razon traduce los motivos habituales que CUPS da en inglés.
func razon(r string) string {
	if r == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "unplugged or turned off":
		r = "desconectada o apagada"
	case "paused":
		r = "en pausa"
	}
	return " (" + r + ")"
}

// uriDispositivo devuelve el device-uri de la cola (usb://…, ipp://…), o "".
func uriDispositivo(ctx context.Context, cola string) string {
	if !reColaValida.MatchString(cola) {
		return ""
	}
	out, _, err := ejecutar(ctx, 3*time.Second, "lpstat", "-v", cola)
	if err != nil {
		return ""
	}
	if i := strings.Index(out, ": "); i >= 0 {
		return strings.TrimSpace(out[i+2:])
	}
	return ""
}

// impresoraConectada dice si la impresora de la cola está presente. Se guía por
// el device-uri de CUPS: si es usb:// busca ese número de serie (o fabricante y
// modelo) en el sistema; si es de red, o no hay URI, da por buena la conexión
// (CUPS ya mostrará el estado).
func impresoraConectada(ctx context.Context, cola string) bool {
	return conectadaPorURI(uriDispositivo(ctx, cola))
}

// conectadaPorURI dice si el dispositivo del device-uri está presente. Los URI
// que no son USB (red) se dan por buenos: CUPS ya informará su estado.
func conectadaPorURI(uri string) bool {
	if uri == "" || !strings.HasPrefix(uri, "usb://") {
		return true
	}
	u, err := url.Parse(uri)
	if err != nil {
		return true
	}
	return usbCoincide(u.Host, strings.TrimPrefix(u.Path, "/"), u.Query().Get("serial"))
}

func dirSysUSB() string {
	if d := os.Getenv("PRINT_SYSUSB"); d != "" {
		return d
	}
	return "/sys/bus/usb/devices"
}

func leerSys(dir, nombre string) string {
	b, _ := os.ReadFile(filepath.Join(dir, nombre))
	return strings.TrimSpace(string(b))
}

// usbCoincide busca la impresora entre los dispositivos USB. Primero por número
// de serie (lo más exacto); como el serie del URI de CUPS a veces sale del
// identificador IEEE-1284 y no del descriptor USB, si ningún dispositivo lo
// tiene se busca por fabricante y modelo (el modelo del URI puede traer «series»).
func usbCoincide(fabricante, modelo, serie string) bool {
	ents, _ := filepath.Glob(filepath.Join(dirSysUSB(), "*", "idVendor"))
	if serie != "" {
		for _, e := range ents {
			if s := leerSys(filepath.Dir(e), "serial"); s != "" && s == serie {
				return true
			}
		}
	}
	fab := strings.ToLower(fabricante)
	mod := strings.ToLower(strings.TrimSuffix(modelo, " series"))
	if fab == "" || mod == "" {
		return false
	}
	for _, e := range ents {
		d := filepath.Dir(e)
		m := strings.ToLower(leerSys(d, "manufacturer"))
		p := strings.ToLower(leerSys(d, "product"))
		if m != "" && strings.Contains(m, fab) && p != "" && strings.Contains(p, mod) {
			return true
		}
	}
	return false
}

// usbPresente quedó sin uso: sin device-uri no se puede saber si la impresora
// está conectada, así que se confía en el estado que reporta CUPS.
