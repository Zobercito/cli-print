package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const version = "0.3.0"

type argumentos struct {
	dry, sinAnim, ascii, reanudar, ver, limpiar, pantalla bool
	copias                                                int
	paginas, papel, tinta, calidad                        string
	ruta                                                  string
}

// parsearArgs admite flags antes y después del archivo (print a.pdf --dry-run).
// Go se detiene en el primer argumento que no es un flag, y eso dejaba
// --dry-run sin efecto e imprimía de verdad.
func parsearArgs(args []string, salida io.Writer) (argumentos, error) {
	var a argumentos
	fs := flag.NewFlagSet("print", flag.ContinueOnError)
	fs.SetOutput(salida)
	fs.BoolVar(&a.dry, "dry-run", false, "muestra la orden lp y no imprime nada")
	fs.BoolVar(&a.sinAnim, "sin-animacion", false, "desactiva las animaciones")
	fs.BoolVar(&a.ascii, "ascii", false, "fuerza el modo ASCII")
	fs.BoolVar(&a.reanudar, "reanudar", false, "reanuda el doble cara pendiente")
	fs.BoolVar(&a.limpiar, "limpiar", false, "borra caché, estado y configuración")
	fs.BoolVar(&a.pantalla, "pantalla-completa", false, "usa la pantalla completa en vez de dibujar en la terminal")
	fs.BoolVar(&a.ver, "version", false, "muestra la versión")
	fs.IntVar(&a.copias, "copias", 1, "(sin terminal) número de copias")
	fs.StringVar(&a.paginas, "paginas", "", "(sin terminal) rango de páginas, ej. 1-3,7")
	fs.StringVar(&a.papel, "papel", "", "(sin terminal) tamaño de papel, ej. Letter, A4")
	fs.StringVar(&a.tinta, "tinta", "", "(sin terminal) color | normal | gris")
	fs.StringVar(&a.calidad, "calidad", "", "(sin terminal) borrador | normal | alta")
	fs.Usage = func() {
		fmt.Fprintln(salida, "uso: print [opciones] [archivo|carpeta|búsqueda]")
		fs.PrintDefaults()
	}
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return a, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	switch len(pos) {
	case 0:
	case 1:
		a.ruta = pos[0]
	default:
		j := strings.Join(pos, " ")
		if _, err := os.Stat(j); err != nil {
			return a, fmt.Errorf("hay %d argumentos; si el nombre tiene espacios, ponlo entre comillas", len(pos))
		}
		a.ruta = j
	}
	return a, nil
}

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) > 1 && subcomandos[os.Args[1]] {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		_, code := subcomando(ctx, os.Args[1:], CargarConfig(), os.Stdout, os.Stderr)
		return code
	}
	a, err := parsearArgs(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "print:", err)
		return 2
	}
	if a.ver {
		fmt.Println("print", version)
		return 0
	}
	if a.limpiar {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Print("Se borrarán la caché, el estado y la configuración de print. ¿Continuar? [s/N] ")
			var r string
			fmt.Scanln(&r)
			if r != "s" && r != "S" && r != "si" && r != "sí" {
				fmt.Println("Cancelado.")
				return 0
			}
		}
		h, _ := os.UserHomeDir()
		for _, d := range []string{".cache/print", ".local/state/print", ".config/print"} {
			os.RemoveAll(h + "/" + d)
		}
		fmt.Println("Caché, estado y configuración borrados.")
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	cfg := CargarConfig()
	anim := cfg.Animacion && !a.sinAnim && os.Getenv("PRINT_ANIMACION") != "0" && os.Getenv("TERM") != "dumb"
	asciiOn := a.ascii || cfg.ASCII || os.Getenv("PRINT_ASCII") == "1" || !localeUTF8()
	defer func() { // no borrar el PDF de un doble cara pendiente (spec 9.2)
		prot := ""
		if s := CargarSesion(); s != nil {
			prot = s.PDF
		}
		LimpiarCache(prot)
	}()

	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return noInteractivo(ctx, cfg, a)
	}
	m := nuevoModelo(ctx, cfg, a.ruta, a.dry, anim, asciiOn)
	if a.reanudar && CargarSesion() == nil {
		fmt.Println("No hay un doble cara pendiente.")
		return 1
	}
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if a.pantalla {
		opts = append(opts, tea.WithAltScreen())
	}
	prog := tea.NewProgram(m, opts...)
	if _, err := prog.Run(); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if m.salida != "" {
		fmt.Println(m.salida)
	}
	return 0
}

// noInteractivo imprime sin menú (spec 12.3). Respeta --dry-run y las opciones
// por flag; el doble cara no está disponible porque exige voltear a mano.
func noInteractivo(ctx context.Context, cfg Config, a argumentos) int {
	fallo := func(msg string, code int) int { fmt.Fprintln(os.Stderr, "print:", msg); return code }
	if a.ruta == "" {
		return fallo("sin terminal interactiva hace falta indicar un archivo", 2)
	}
	if a.copias < 1 || a.copias > 99 {
		return fallo("--copias debe estar entre 1 y 99", 2)
	}
	tinta := cfg.Tinta
	if a.tinta != "" {
		if _, ok := colorModel[a.tinta]; !ok {
			return fallo("--tinta debe ser color, normal o gris", 2)
		}
		tinta = a.tinta
	}
	calidad := cfg.Calidad
	if a.calidad != "" {
		if _, ok := calidadModel[a.calidad]; !ok {
			return fallo("--calidad debe ser borrador, normal o alta", 2)
		}
		calidad = a.calidad
	}
	papel := papelDeCola(ctx, cfg.Impresora)
	if papel == "" {
		papel = "Letter"
	}
	if a.papel != "" {
		papeles := TamanosPapel(cfg.Impresora)
		if indiceDe(papeles, a.papel) < 0 {
			return fallo("papel desconocido: "+a.papel+" (válidos: "+strings.Join(papeles, " ")+")", 2)
		}
		papel = a.papel
	}
	if st, err := os.Stat(a.ruta); err == nil && st.IsDir() {
		return fallo("eso es una carpeta; indica un archivo (o usa print sin terminal solo con archivos)", 2)
	}
	if msg := Validar(a.ruta); msg != nil {
		return fallo(msg.Texto(), 1)
	}
	pdf, err := Convertir(ctx, a.ruta, 60e9)
	if err != nil {
		return fallo(err.Error(), 1)
	}
	info, err := AnalizarPDF(ctx, pdf)
	if err != nil {
		return fallo(err.Error(), 1)
	}
	var pags []int
	if a.paginas != "" {
		if pags, err = ParseRangos(a.paginas, info.Paginas); err != nil {
			return fallo(err.Error(), 2)
		}
	}
	o := Opciones{Cola: cfg.Impresora, Papel: papel, Tinta: tinta, Calidad: calidad, Orientacion: "Portrait", Copias: a.copias, Paginas: pags, Orden: "reverse"}
	if info.Ancho > 0 && (info.Papel != papel || info.Ancho > info.Alto+1) {
		o.Escala = "fit" // igual que el menú: Ajustar por defecto
	}
	args := ConstruirLP(o, pdf)
	if a.dry {
		fmt.Println("lp " + shellMostrar(args))
		return 0
	}
	id, err := Enviar(ctx, args)
	if err != nil {
		if e, ok := err.(*ErrLP); ok {
			m := TraducirErrorLP(e.Stderr)
			return fallo(m.Texto(), 1)
		}
		return fallo(err.Error(), 1)
	}
	RegistrarEnvio(id, filepath.Base(a.ruta))
	fmt.Println("Enviado:", id)
	return 0
}
