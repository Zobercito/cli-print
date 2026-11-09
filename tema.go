package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	cAcento = lipgloss.Color("#7DCFFF")
	cViol   = lipgloss.Color("#BB9AF7")
	cOk     = lipgloss.Color("#9ECE6A")
	cAviso  = lipgloss.Color("#E0AF68")
	cError  = lipgloss.Color("#F7768E")
	cTenue  = lipgloss.Color("#7A84B5")

	sAcento = lipgloss.NewStyle().Foreground(cAcento)
	sViol   = lipgloss.NewStyle().Foreground(cViol)
	sOk     = lipgloss.NewStyle().Foreground(cOk)
	sAviso  = lipgloss.NewStyle().Foreground(cAviso)
	sError  = lipgloss.NewStyle().Foreground(cError)
	sTenue  = lipgloss.NewStyle().Foreground(cTenue)
	sNeg    = lipgloss.NewStyle().Bold(true)
)

// Simbolos agrupa los glifos; hay una variante ASCII (spec 10.6).
type Simbolos struct {
	Sel, On, Off, Ok, Mal, Aviso, Info, Imp, Doc, Giro string
	Spin                                               []string
	Esq                                                [4]string // ╭ ╮ ╰ ╯
	H, V, TL, TR                                       string
	Hoja                                               []string
	Casilla, CasillaOn                                 string
}

var uni = Simbolos{"▸", "◉", "○", "✓", "✗", "⚠", "ⓘ", "⎙", "▤", "⟳",
	[]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	[4]string{"╭", "╮", "╰", "╯"}, "─", "│", "├", "┤", nil, "☐", "☑"}

var asc = Simbolos{">", "(*)", "( )", "OK", "X", "!", "i", "[P]", "[D]", "@",
	[]string{"|", "/", "-", "\\"},
	[4]string{"+", "+", "+", "+"}, "-", "|", "+", "+", nil, "[ ]", "[x]"}

func elegirSimbolos(ascii bool) Simbolos {
	if ascii {
		return asc
	}
	return uni
}

func localeUTF8() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return false
}

// degradado reparte un color entre a y b a lo largo del texto.
func degradado(s string, a, b [3]int) string {
	r := []rune(s)
	var sb strings.Builder
	for i, c := range r {
		t := 0.0
		if len(r) > 1 {
			t = float64(i) / float64(len(r)-1)
		}
		col := fmt.Sprintf("#%02x%02x%02x",
			int(float64(a[0])+t*float64(b[0]-a[0])),
			int(float64(a[1])+t*float64(b[1]-a[1])),
			int(float64(a[2])+t*float64(b[2]-a[2])))
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(col)).Render(string(c)))
	}
	return sb.String()
}

func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w > n {
		return ansi.Truncate(s, n, "…")
	}
	return s + strings.Repeat(" ", n-w)
}

// marco dibuja la caja a mano para poder animar su apertura (p de 0 a 1) y
// sacudirla (desp columnas a la izquierda) sin depender del borde de lipgloss.
func marco(sim Simbolos, titulo string, cuerpo []string, pie string, W int, p float64, desp int) string {
	inner := W - 4
	if p < 1 {
		inner = max(4, int(float64(inner)*p))
		cuerpo = cuerpo[:int(float64(len(cuerpo))*p)]
	}
	borde := sTenue
	l := func(s string) string { return borde.Render(sim.V) + " " + pad(s, inner) + " " + borde.Render(sim.V) }
	var f []string
	t := " " + titulo + " "
	rest := inner + 2 - 1 - lipgloss.Width(t)
	if rest < 0 {
		rest = 0
	}
	f = append(f, borde.Render(sim.Esq[0]+sim.H)+t+borde.Render(strings.Repeat(sim.H, rest)+sim.Esq[1]))
	for _, c := range cuerpo {
		f = append(f, l(c))
	}
	if pie != "" && p >= 1 {
		f = append(f, borde.Render(sim.TL+strings.Repeat(sim.H, inner+2)+sim.TR))
		f = append(f, l(pie))
	}
	f = append(f, borde.Render(sim.Esq[2]+strings.Repeat(sim.H, inner+2)+sim.Esq[3]))
	out := strings.Join(f, "\n")
	if desp != 0 {
		pre := strings.Repeat(" ", max(0, 2+desp))
		out = pre + strings.ReplaceAll(out, "\n", "\n"+pre)
	}
	return out
}
