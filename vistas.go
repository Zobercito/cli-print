package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) View() string {
	if m.fin && !m.dejar {
		return ""
	}
	if m.w < 60 || m.h < 24 {
		return sAviso.Render(fmt.Sprintf("Agranda la terminal (mínimo 60×24, ahora %d×%d)", m.w, m.h))
	}
	W := min(78, m.w-2)
	inner := W - 4
	var tit, pie string
	var cuerpo []string
	switch m.pant {
	case pArranque:
		tit, cuerpo, pie = m.vArranque()
	case pReanudar:
		tit, cuerpo, pie = m.vReanudar(inner)
	case pBuscador:
		tit, cuerpo, pie = m.vBuscador(inner)
	case pAnalizando:
		tit, cuerpo, pie = m.vAnalizando()
	case pMenu:
		tit, cuerpo, pie = m.vMenu(inner)
	case pVista:
		tit, cuerpo, pie = m.vVista(inner)
	case pImprimiendo:
		tit, cuerpo, pie = m.vImprimiendo()
	case pVolteo:
		tit, cuerpo, pie = m.vVolteo(inner)
	case pListo:
		tit, cuerpo, pie = m.vListo(inner)
	case pError:
		tit, cuerpo, pie = m.vError(inner)
	}
	p := 1.0
	desp := 0
	if time.Now().Before(m.shake) && m.anim {
		desp = []int{-2, 2}[(m.frame/2)%2]
	}
	caja := marco(m.sim, degradado("print", [3]int{125, 207, 255}, [3]int{187, 154, 247})+" "+sTenue.Render(tit),
		cuerpo, sTenue.Render(pie), W, p, desp)
	return caja
}

func (m *model) spin() string { return sAcento.Render(m.sim.Spin[(m.frame/2)%len(m.sim.Spin)]) }

func (m *model) radio(fila int, vals []string, sel int) string {
	var p []string
	for i, v := range vals {
		if i == sel {
			st := sAcento.Bold(true)
			if fila == m.fila && time.Now().Before(m.pulso) {
				st = sViol.Bold(true)
			}
			p = append(p, st.Render(m.sim.On+" "+v))
		} else {
			p = append(p, sTenue.Render(m.sim.Off+" "+v))
		}
	}
	return strings.Join(p, "  ")
}

func (m *model) etiqueta(fila int, txt string) string {
	if fila == m.fila {
		return sAcento.Render(m.sim.Sel+" ") + sNeg.Render(pad(txt, 12))
	}
	return "  " + pad(txt, 12)
}

func ajustar(s string, n int) []string {
	return strings.Split(lipgloss.NewStyle().Width(n).Render(s), "\n")
}

func (m *model) vArranque() (string, []string, string) {
	return "", []string{"", "  " + m.spin() + " Revisando impresora…", "", sTenue.Render("  CUPS · cola · USB · conversor · fuentes"), ""}, ""
}

func (m *model) vReanudar(inner int) (string, []string, string) {
	s := m.ses
	c := []string{"", "  " + sAviso.Render(m.sim.Aviso) + " Hay un doble cara pendiente", "",
		"  Archivo   " + filepath.Base(s.Archivo),
		fmt.Sprintf("  Falta     pasada 2 (%d páginas del reverso)", len(s.R)),
		fmt.Sprintf("  Guardada  %s", s.Guardada.Format("02/01 15:04")), ""}
	return "reanudar", c, "Enter reanudar   d descartar   q salir"
}

func (m *model) vBuscador(inner int) (string, []string, string) {
	c := []string{m.busq.View(), ""}
	// La lista muestra como mucho 10 documentos; el resto se alcanza bajando.
	vis := min(10, max(5, m.h-12))
	ini := 0
	if m.cur >= vis {
		ini = m.cur - vis + 1
	}
	if len(m.archivos) == 0 {
		c = append(c, "  "+m.spin()+sTenue.Render(" buscando documentos…"))
	} else if len(m.coin) == 0 {
		c = append(c, sTenue.Render("  sin resultados"))
	}
	h, _ := os.UserHomeDir()
	for i := ini; i < len(m.coin) && i < ini+vis; i++ {
		co := m.coin[i]
		nom := []rune(co.Nombre)
		if len(nom) > 46 {
			nom = append(nom[:45], '…')
		}
		set := map[int]bool{}
		for _, p := range co.Pos {
			set[p] = true
		}
		var sb strings.Builder
		for j, r := range nom {
			if set[j] {
				sb.WriteString(sViol.Bold(true).Render(string(r)))
			} else {
				sb.WriteRune(r)
			}
		}
		dir := strings.Replace(filepath.Dir(co.Ruta), h, "~", 1)
		linea := pad(sb.String(), 47) + sTenue.Render(pad(dir, inner-52))
		if i == m.cur {
			c = append(c, sAcento.Render(m.sim.Sel+" ")+sNeg.Render(linea))
		} else {
			c = append(c, "  "+linea)
		}
	}
	return "buscar documento", c, "↑↓ mover   Enter elegir   esc salir"
}

var msgsConv = []string{"Abriendo documento…", "Maquetando páginas…", "Generando PDF…"}

func (m *model) vAnalizando() (string, []string, string) {
	t := "Leyendo PDF…"
	if !strings.EqualFold(filepath.Ext(m.ruta), ".pdf") {
		t = "Convirtiendo a PDF… " + sTenue.Render(msgsConv[(m.frame/25)%len(msgsConv)])
	}
	w := 30
	pos := (m.frame / 2) % (2 * (w - 6))
	if pos > w-6 {
		pos = 2*(w-6) - pos
	}
	barra := strings.Repeat("░", pos) + "██████" + strings.Repeat("░", w-6-pos)
	if m.sim.On == "(*)" {
		barra = strings.Repeat(".", pos) + "######" + strings.Repeat(".", w-6-pos)
	}
	return "analizando", []string{"", "  " + m.spin() + " " + t, "", "  " + sViol.Render(barra), "", sTenue.Render("  " + filepath.Base(m.ruta)), ""}, "esc cancelar"
}

func (m *model) vMenu(inner int) (string, []string, string) {
	var c []string
	if !m.descartado {
		for _, l := range m.banda() {
			c = append(c, sAviso.Render(m.sim.Aviso+" ")+l)
		}
	}
	punto := sOk.Render("● Lista")
	if m.problemaImprimirCola() {
		punto = sError.Render("● Detenida")
	}
	c = append(c, sAcento.Render(m.sim.Imp)+" "+sNeg.Render("Impresora")+"   "+punto, "")
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(m.ruta), "."))
	meta := ext
	if m.info.Paginas > 0 {
		meta += fmt.Sprintf(" · %d páginas", m.info.Paginas)
	}
	meta += " · " + humano(m.tam)
	if ext == "MD" || ext == "MARKDOWN" || ext == "TXT" {
		meta += " · sin formato"
	}
	if m.info.Papel != "" {
		meta += " · " + m.info.Papel
	}
	c = append(c, " "+sAcento.Render(m.sim.Doc)+" "+sNeg.Render(filepath.Base(m.ruta)), "    "+sTenue.Render(meta), "")

	var tv, mv []string
	for _, t := range tintas {
		tv = append(tv, t.Txt)
	}
	for _, o := range medios {
		mv = append(mv, o.Txt)
	}
	ov := []string{orients[0].Txt, orients[1].Txt}
	cv := []string{calidades[0].Txt, calidades[1].Txt, calidades[2].Txt}
	rango := m.radio(rPaginas, []string{"Todas", "Rango"}, b2i(m.modoRango))
	if m.modoRango {
		cur := " "
		if (m.frame/8)%2 == 0 && m.fila == rPaginas {
			cur = "▏"
			if m.sim.On == "(*)" {
				cur = "_"
			}
		}
		rango += " " + sViol.Render("["+m.rangoTxt+cur+"]")
	}
	c = append(c,
		m.etiqueta(rTinta, "Tinta")+m.radio(rTinta, tv, m.iTinta),
		m.etiqueta(rCalidad, "Calidad")+m.radio(rCalidad, cv, m.iCalidad),
		m.etiqueta(rPapel, "Papel")+m.filaPapel(),
		m.filaEscala(),
		m.etiqueta(rCaras, "Caras")+m.radio(rCaras, []string{"Una", "Doble (manual)"}, m.caras),
		m.etiqueta(rPaginas, "Páginas")+rango,
		m.etiqueta(rCopias, "Copias")+sAcento.Render("‹ ")+sNeg.Render(fmt.Sprint(m.copias))+sAcento.Render(" ›"),
		m.etiqueta(rOrient, "Orientación")+m.radio(rOrient, ov, m.iOrient),
		m.etiqueta(rMedio, "Tipo papel")+m.radio(rMedio, mv, m.iMedio),
		m.etiqueta(rBordes, "Sin bordes")+m.radio(rBordes, []string{"No", "Sí"}, b2i(m.bordes)), "")

	var sin []string
	for _, l := range c {
		if l != "\x00" {
			sin = append(sin, l)
		}
	}
	c = sin
	if m.nota != "" {
		c = append(c, sAviso.Render(m.sim.Info+" ")+m.nota)
	}
	if p := m.problemaSeleccion(); p == "" {
		c = append(c, sTenue.Render(m.sim.Info+" ")+m.resumen())
	} else if m.modoRango && strings.TrimSpace(m.rangoTxt) == "" {
		c = append(c, sTenue.Render(m.sim.Info+" Escribe un rango, por ejemplo 1-3,7"))
	} else {
		c = append(c, sError.Render(m.sim.Mal+" "+p))
	}
	return "menú", c, "↑↓ mover  ←→ cambiar  Enter imprimir  r recargar  esc atrás  q salir"
}

// banda resume los avisos en pocas líneas: un solo mensaje sobre la impresora
// (el más específico) y, como mucho, un aviso menor más.
func (m *model) banda() []string {
	var out []string
	var usb, deshab, acepta *Hallazgo
	var menor *Hallazgo
	for i := range m.hallazgos {
		h := &m.hallazgos[i]
		switch h.ID {
		case "P-6":
			usb = h
		case "P-4", "P-7":
			deshab = h
		case "P-5":
			acepta = h
		default:
			if menor == nil {
				menor = h
			}
		}
	}
	switch {
	case usb != nil:
		out = append(out, "Impresora desconectada o apagada"+sTenue.Render(" · conéctala y pulsa r"))
	case deshab != nil:
		ayuda := accionCorta(deshab.Accion)
		if m.colaDetenida() {
			ayuda = " · e habilitar"
		}
		out = append(out, deshab.Que+sTenue.Render(ayuda))
	case acepta != nil:
		out = append(out, acepta.Que+sTenue.Render(accionCorta(acepta.Accion)))
	}
	if menor != nil {
		out = append(out, menor.Que+sTenue.Render(accionCorta(menor.Accion)))
	}
	// Solo lo que espera: el trabajo que ya se está imprimiendo no necesita aviso.
	var esperan []Trabajo
	for _, t := range m.trabajosCola {
		if t.Estado != "imprimiendo" {
			esperan = append(esperan, t)
		}
	}
	if n := len(esperan); n > 0 {
		viejo := int64(-1)
		for _, t := range esperan {
			if t.EdadS > viejo {
				viejo = t.EdadS
			}
		}
		txt := pluralTrabajos(n) + " en cola"
		if viejo >= 0 {
			txt += " (el más antiguo: " + edadHumana(viejo) + ")"
		}
		if m.propiosEnCola() > 0 {
			if m.confirmCancel {
				txt = "¿Cancelar " + pluralTrabajos(m.propiosEnCola()) + "? Pulsa x otra vez"
			} else {
				txt += sTenue.Render(" · x cancelarlos")
			}
		}
		out = append(out, txt)
	}
	return out
}

func accionCorta(a string) string {
	if a == "" {
		return ""
	}
	return " · " + a
}

func (m *model) filaEscala() string {
	if !m.hayDesajuste() {
		return "\x00" // se filtra en vMenu
	}
	if m.escalaAbierta {
		return m.etiqueta(rEscala, "Escala") + sAcento.Render("‹ ") + sViol.Bold(true).Render(escalas[m.escalaPick].Txt) +
			sAcento.Render(" ›") + sTenue.Render("  ←→ elegir · Enter confirmar · esc cancelar")
	}
	vis := m.escalasVisibles()
	var v []string
	for _, i := range vis {
		v = append(v, escalas[i].Txt)
	}
	v = append(v, "Más")
	return m.etiqueta(rEscala, "Escala") + m.radio(rEscala, v, posEn(vis, m.iEscala))
}

func (m *model) filaPapel() string {
	if m.papelAbierto {
		return sAcento.Render("‹ ") + sViol.Bold(true).Render(etiquetaPapel(m.todosPapeles[m.papelPick])) +
			sAcento.Render(" ›") + sTenue.Render("  ←→ elegir · Enter confirmar · esc cancelar")
	}
	var v []string
	for _, p := range m.papelVis {
		v = append(v, etiquetaPapel(p))
	}
	v = append(v, "Más")
	return m.radio(rPapel, v, indiceDe(m.papelVis, m.papel))
}

func (m *model) problemaImprimirCola() bool {
	for _, h := range m.hallazgos {
		if h.Grave {
			return true
		}
	}
	return false
}

func (m *model) resumen() string {
	sel, err := m.seleccion()
	if err != nil {
		return "revisa el rango"
	}
	n := len(sel)
	if n == 0 {
		n = m.info.Paginas
	}
	tinta := strings.ToLower(tintas[m.iTinta].Txt)
	if m.usaDuplex() && n > 1 {
		pl := PlanificarDuplex(sel)
		s := fmt.Sprintf("%d hojas, 2 pasadas, %s %s, %s", pl.Hojas, etiquetaPapel(m.papel), strings.ToLower(orients[m.iOrient].Txt), tinta)
		if pl.SinReverso > 0 {
			s += fmt.Sprintf(" · la pág. %d queda sin reverso", pl.SinReverso)
		}
		if m.copias > 1 {
			s += fmt.Sprintf(" · %d copias (un ciclo por copia)", m.copias)
		}
		return s
	}
	if n == 0 {
		return fmt.Sprintf("%d copia(s), %s", m.copias, tinta)
	}
	return fmt.Sprintf("%d páginas × %d copia(s), %s %s, %s", n, m.copias, etiquetaPapel(m.papel), strings.ToLower(orients[m.iOrient].Txt), tinta)
}

func (m *model) vVista(inner int) (string, []string, string) {
	c := []string{""}
	n := 1
	if m.usaDuplex() {
		n = 2
	}
	for p := 1; p <= n; p++ {
		if n == 2 {
			tit := fmt.Sprintf("Pasada %d de 2", p)
			if p == 2 {
				tit += " (después de voltear las hojas)"
			}
			c = append(c, sNeg.Render(tit))
		}
		for _, l := range m.lineasResumen(p) {
			c = append(c, "  "+l)
		}
		c = append(c, "")
	}
	if m.usaDuplex() && !m.blancoAuto {
		if sel, _ := m.seleccion(); len(sel)%2 == 1 {
			for _, l := range ajustar(fmt.Sprintf("La página %d va sola. Al voltear, pon una hoja en blanco encima del bloque.", sel[len(sel)-1]), inner-2) {
				c = append(c, sTenue.Render(l))
			}
			c = append(c, "")
		}
	}
	if m.verOrden {
		c = append(c, sTenue.Render("Orden que se ejecutará:"))
		for p := 1; p <= n; p++ {
			c = append(c, m.envolver("lp "+shellMostrar(m.argumentos(p)), inner-4)...)
		}
		c = append(c, "")
	}
	if m.editando {
		c = append(c, m.edit.View())
		if m.editErr != "" {
			c = append(c, ajustar(sError.Render(m.editErr), inner)...)
		}
	} else if len(m.extra) > 0 {
		c = append(c, sTenue.Render("opciones extra: "+strings.Join(m.extra, " ")))
	}
	if m.msgCopia != "" {
		c = append(c, sOk.Render(m.sim.Ok+" "+m.msgCopia))
	}
	if m.dupAviso != "" {
		c = append(c, sAviso.Render(m.sim.Aviso+" ")+m.dupAviso)
	}
	if m.dry {
		c = append(c, sAviso.Render(m.sim.Aviso+" modo --dry-run: no se enviará nada"))
	}
	pie := "Enter ENVIAR a la impresora   o ver orden   esc volver"
	if m.editando {
		pie = "Enter aplicar   esc cancelar"
	}
	return "vista previa", c, pie
}

// nombreVisible es el nombre que se muestra en el detalle; nunca queda vacío.
func (m *model) nombreVisible() string {
	if m.nombre != "" {
		return m.nombre
	}
	return "Impresora"
}

// lineasResumen describe en palabras lo que se va a imprimir en una pasada.
func (m *model) lineasResumen(pasada int) []string {
	f := func(etq, val string) string { return sTenue.Render(pad(etq, 13)) + val }
	sel, _ := m.seleccion()
	var out []string
	if pasada == 1 { // la pasada 2 solo muestra lo que cambia
		out = append(out, f("Archivo", filepath.Base(m.ruta)), f("Impresora", m.nombreVisible()),
			f("Papel", etiquetaPapel(m.papel)), f("Tinta", tintas[m.iTinta].Txt))
	}
	orient := strings.ToLower(orients[m.iOrient].Txt)
	if m.usaDuplex() && pasada == 1 && m.reversoRotado() {
		orient += ", girada 180°"
	}
	out = append(out, f("Orientación", orient))
	paginas := "todas"
	switch {
	case m.usaDuplex():
		pl := PlanificarDuplex(sel)
		if pasada == 1 {
			paginas = listaOrden(pl.Pasada1) + " (primero las caras de atrás)"
			if m.blancoAuto {
				paginas = "hoja en blanco, " + listaOrden(pl.Pasada1) + " (primero las caras de atrás)"
			}
		} else {
			paginas = listaOrden(pl.Pasada2) + " (las caras de adelante)"
		}
	case m.modoRango:
		paginas = FormatoRangos(sel)
	}
	out = append(out, f("Páginas", paginas))
	if !m.usaDuplex() {
		out = append(out, f("Copias", fmt.Sprint(m.copias)))
	}
	if pasada == 1 {
		out = append(out, f("Caras", map[bool]string{true: "doble, manual (2 pasadas)", false: "una"}[m.usaDuplex()]))
	}
	if pasada == 1 && m.hayDesajuste() {
		e := escalas[m.iEscala].Txt
		if m.apaisado() {
			e += " · el PDF es apaisado"
		}
		out = append(out, f("Escala", e))
	}
	if pasada == 1 && medios[m.iMedio].Val != "" {
		out = append(out, f("Tipo papel", medios[m.iMedio].Txt))
	}
	if pasada == 1 && m.bordes {
		out = append(out, f("Sin bordes", "sí"))
	}
	return out
}

func (m *model) envolver(s string, n int) []string {
	var out []string
	linea := "  "
	for _, w := range strings.Fields(s) {
		if len(linea)+len(w)+3 > n && strings.TrimSpace(linea) != "" {
			out = append(out, sAcento.Render(linea+"\\"))
			linea = "     "
		}
		linea += w + " "
	}
	return append(out, sAcento.Render(strings.TrimRight(linea, " ")))
}

func (m *model) vImprimiendo() (string, []string, string) {
	n := (m.frame / 4) % 5
	c := []string{"", "  " + m.spin() + " " + sNeg.Render(m.textoJob)}
	det := "  " + filepath.Base(m.ruta)
	if m.usaDuplex() {
		det += fmt.Sprintf(" · pasada %d de 2", m.pasada)
		if m.copias > 1 {
			det += fmt.Sprintf(" · copia %d de %d", m.copia, m.copias)
		}
	}
	c = append(c, sTenue.Render(det), "")
	c = append(c, sAcento.Render("      ╭─────────────╮"), sAcento.Render("      │ ")+sViol.Render(m.sim.Imp)+sAcento.Render("           │"), sAcento.Render("      ╰──╥───────╥──╯"))
	if m.sim.On == "(*)" {
		c[len(c)-3], c[len(c)-2], c[len(c)-1] = "      +-------------+", "      | [P]         |", "      +--||-----||--+"
	}
	for i := 0; i < n; i++ {
		c = append(c, sTenue.Render("         ║")+"▒▒▒▒▒▒▒"+sTenue.Render("║"))
	}
	for i := n; i < 4; i++ {
		c = append(c, "")
	}
	return "imprimiendo", c, "c cancelar este trabajo   q salir (sigue en la cola)"
}

// secuenciaGiro: tamaño de la hoja en cada fotograma (se aplana hasta una línea
// y vuelve a abrirse). Pasado el punto medio los bordes han cambiado de lado.
var secuenciaGiro = []int{5, 5, 5, 5, 4, 3, 2, 1, 2, 3, 4, 5, 5, 5, 5}

const giroMitad = 7

// animHoja dibuja la hoja girando. «tortita»: se levanta el borde que mira
// hacia ti y se pasa hacia atrás (la hoja se aplana en vertical y el borde de
// arriba termina hacia ti). «libro»: como pasar una página (se aplana en horizontal).
func (m *model) animHoja() []string {
	i := 0
	if m.anim {
		i = (m.frame / 3) % len(secuenciaGiro)
	}
	n := secuenciaGiro[i]
	cambiado := i > giroMitad
	arriba, abajo := sAcento, sViol // arriba = borde que estaba ARRIBA; abajo = el que estaba hacia TI
	if cambiado {
		arriba, abajo = abajo, arriba
	}
	cuerpo, bT, bB := "▒", "▀", "▄"
	if m.sim.On == "(*)" {
		cuerpo, bT, bB = "#", "=", "-"
	}
	const ancho = 14
	margen := strings.Repeat(" ", 6)
	out := make([]string, 5)
	if m.cfg.Giro != "tortita" { // libro: se gira por un borde largo (los de los lados)
		const H, W = 16, 32 // la bisagra (el borde por el que giras) queda en la columna H
		w := []int{14, 14, 14, 14, 11, 8, 5, 2, 5, 8, 11, 14, 14, 14, 14}[i]
		izq, der, cuerpoC := "▌", "▐", "▒"
		if m.sim.On == "(*)" {
			izq, der, cuerpoC = "|", "|", "#"
		}
		celdas := make([]string, W)
		for k := range celdas {
			celdas[k] = " "
		}
		celdas[H] = sTenue.Render("┆")
		if m.sim.On == "(*)" {
			celdas[H] = sTenue.Render(":")
		}
		if !cambiado { // la hoja está a la izquierda de la bisagra
			for k := H - w; k < H; k++ {
				celdas[k] = cuerpoC
			}
			celdas[H-w] = sAcento.Render(izq)
			celdas[H-1] = sViol.Render(der)
		} else { // ya pasó al otro lado
			for k := H; k < H+w; k++ {
				celdas[k] = cuerpoC
			}
			celdas[H] = sViol.Render(izq)
			celdas[H+w-1] = sAcento.Render(der)
		}
		fila := strings.Join(celdas, "")
		for r := range out {
			out[r] = "  " + fila
		}
		out[1] += "  " + sViol.Render(der) + " borde por el que giras (queda fijo)"
		out[3] += "  " + sAcento.Render(izq) + " borde opuesto (pasa al otro lado)"
		return out
	}
	// tortita: n filas visibles centradas en 5
	ini := (5 - n) / 2
	for r := range out {
		out[r] = margen
	}
	for r := 0; r < n; r++ {
		var fila string
		switch {
		case n == 1:
			fila = sTenue.Render(strings.Repeat("━", ancho))
			if m.sim.On == "(*)" {
				fila = strings.Repeat("~", ancho)
			}
		case r == 0:
			fila = arriba.Render(strings.Repeat(bT, ancho))
		case r == n-1:
			fila = abajo.Render(strings.Repeat(bB, ancho))
		default:
			fila = strings.Repeat(cuerpo, ancho)
		}
		out[ini+r] = margen + fila
	}
	out[0] += "   " + sAcento.Render(bT) + " el borde que estaba ARRIBA"
	out[4] += "   " + sViol.Render(bB) + " el borde que estaba hacia TI"
	return out
}

func (m *model) vVolteo(inner int) (string, []string, string) {
	c := []string{"", "  " + sOk.Render(m.sim.Ok) + " Pasada 1 de 2 terminada", ""}
	c = append(c, m.animHoja()...)
	c = append(c, "")
	t := "Gira el bloque por un borde largo, como la página de un libro, y ponlo otra vez en la bandeja."
	if m.cfg.Giro == "tortita" {
		t = "Gira el bloque levantando el borde que mira hacia ti, como la hoja de un calendario, y ponlo otra vez en la bandeja."
	}
	for _, l := range ajustar(t, inner-4) {
		c = append(c, "  "+l)
	}
	if m.plan.SinReverso > 0 && !m.blancoAuto { // si print añade la hoja solo, no hay nada que decir
		c = append(c, "")
		for i, l := range ajustar(fmt.Sprintf("Son páginas impares: añade una hoja en blanco encima del bloque (la página %d sale sola).", m.plan.SinReverso), inner-6) {
			pre := "  " + sTenue.Render(m.sim.Info) + " "
			if i > 0 {
				pre = "    "
			}
			c = append(c, pre+l)
		}
	}
	c = append(c, "")
	return "voltea las hojas", c, "Enter imprimir la pasada 2   g girar reverso   q guardar y salir"
}

func (m *model) vListo(inner int) (string, []string, string) {
	st := sOk.Bold(true)
	if time.Since(m.entrada) < 450*time.Millisecond && (m.frame/3)%2 == 1 {
		st = sNeg
	}
	c := []string{"", "  " + st.Render(m.sim.Ok+" Listo"), ""}
	if m.dry {
		c = append(c, "  "+sAviso.Render("--dry-run: no se envió nada"))
	} else {
		for _, l := range m.resumenImpreso() {
			c = append(c, "  "+l)
		}
	}
	return "listo", append(c, ""), "Enter salir   n otra impresión"
}

// resumenImpreso cuenta en palabras lo que se imprimió (sin códigos de trabajo).
func (m *model) resumenImpreso() []string {
	f := func(etq, val string) string { return sTenue.Render(pad(etq, 11)) + val }
	sel, _ := m.seleccion()
	n := len(sel)
	paginas := "todas"
	switch {
	case m.modoRango && n > 0:
		paginas = FormatoRangos(sel)
	case n > 0:
		paginas = fmt.Sprintf("todas (%d)", n)
	}
	out := []string{f("Archivo", filepath.Base(m.ruta)), f("Páginas", paginas)}
	if n > 0 {
		hojas := n * m.copias
		forma := "una cara"
		if m.usaDuplex() {
			hojas, forma = ((n+1)/2)*m.copias, "doble cara"
		}
		txt := fmt.Sprintf("%d (%s)", hojas, forma)
		if hojas == 1 {
			txt = fmt.Sprintf("1 (%s)", forma)
		}
		out = append(out, f("Hojas", txt))
	}
	if m.copias > 1 {
		out = append(out, f("Copias", fmt.Sprint(m.copias)))
	}
	out = append(out, f("Papel", etiquetaPapel(m.papel)), f("Tinta", tintas[m.iTinta].Txt))
	if m.hayDesajuste() {
		out = append(out, f("Escala", escalas[m.iEscala].Txt))
	}
	return out
}

func (m *model) vError(inner int) (string, []string, string) {
	c := []string{""}
	if m.err != nil {
		c = append(c, ajustar(sError.Render(m.sim.Mal+" ")+sNeg.Render(m.err.Que), inner-2)...)
		if m.err.Accion != "" {
			c = append(c, "", "  "+sAcento.Render(m.err.Accion))
		}
	}
	if m.errOrig != "" {
		c = append(c, "")
		for _, l := range ajustar(m.errOrig, inner-4) {
			c = append(c, "  "+sTenue.Render(l))
		}
	}
	if m.msgCopia != "" {
		c = append(c, "", "  "+sOk.Render(m.msgCopia))
	}
	pie := "Enter reintentar   q salir"
	if m.err != nil && strings.HasPrefix(m.err.Accion, "sudo ") {
		pie = "c copiar comando   Enter reintentar   q salir"
	}
	return "error", append(c, ""), pie
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func humano(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// listaOrden: "4, 2" respetando el orden (FormatoRangos lo ordenaría).
func listaOrden(l []int) string {
	p := make([]string, len(l))
	for i, v := range l {
		p[i] = fmt.Sprint(v)
	}
	return strings.Join(p, ", ")
}
