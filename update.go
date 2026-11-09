package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = v.Width, v.Height
		ancho := max(10, min(60, v.Width-14))
		m.busq.Width, m.edit.Width = ancho, ancho
		return m, nil
	case tickMsg:
		m.frame++
		if (m.pant == pMenu || m.pant == pBuscador || m.pant == pVista) && !m.autoBusy &&
			time.Since(m.autoUlt) > 3*time.Second && (m.problemaImprimirCola() || len(m.trabajosCola) > 0) {
			m.autoBusy, m.autoUlt = true, time.Now()
			return m, tea.Batch(m.tick(), cmdAuto(m.ctx, m.cola))
		}
		return m, m.tick()
	case msgPre:
		m.cola, m.hallazgos, m.trabajosCola = v.cola, v.hs, v.trabajos
		for _, h := range v.hs {
			if h.Bloqueante {
				m.err, m.volver = &h.Mensaje, pArranque
				m.setPant(pError)
				return m, nil
			}
		}
		if m.recargando && m.pdf != "" { // «r» desde el menú: volver al menú, no al buscador
			m.recargando = false
			m.setPant(pMenu)
			return m, nil
		}
		m.recargando = false
		if s := CargarSesion(); s != nil && s.Estado == "ESPERA_VOLTEO" {
			m.ses = s
			m.setPant(pReanudar)
			return m, nil
		}
		return m, m.irABuscar()
	case msgAuto:
		m.autoBusy = false
		m.cola, m.hallazgos, m.trabajosCola = v.cola, v.hs, v.trabajos
		return m, nil
	case msgHabilitado:
		if v.err != nil {
			m.nota = "No se pudo habilitar: " + v.err.Error()
		} else {
			m.nota = "Cola habilitada"
		}
		return m, cmdAuto(m.ctx, m.cola)
	case msgCancelado:
		m.confirmCancel = false
		if len(v.errs) > 0 {
			m.nota = "No se pudo cancelar: " + resumenErrores(v.errs)
			if a := accionDe(v.errs); a != "" {
				m.nota += "  →  " + a
			}
		} else {
			m.nota = "Trabajos cancelados"
		}
		if v.pasada && m.pant == pImprimiendo {
			m.setPant(pMenu)
		}
		return m, cmdAuto(m.ctx, m.cola)
	case msgArchivos:
		m.archivos = []Archivo(v)
		m.coin = Buscar(m.archivos, m.busq.Value())
		return m, nil
	case msgAnalizado:
		if v.id != m.analID {
			return m, nil // respuesta obsoleta (13.3)
		}
		if v.err != nil {
			msg := Mensaje{"No pude preparar el documento.", ""}
			switch {
			case v.err == ErrPDFCifrado:
				msg = Mensaje{"El PDF está protegido con contraseña.", "Desbloquéalo y vuelve a intentar"}
			case v.err == ErrPDFDanado:
				msg = Mensaje{"El PDF está dañado o incompleto.", "Ábrelo en otro programa y guárdalo de nuevo como PDF"}
			case v.err == ErrPDFSinPermiso:
				msg = Mensaje{"El PDF no permite imprimir.", "Pide una copia sin esa restricción"}
			case strings.Contains(v.err.Error(), "tardó"):
				msg.Accion = "Pulsa Enter para reintentar"
			}
			m.errOrig = v.err.Error()
			m.err, m.volver = &msg, pBuscador
			m.shake = time.Now().Add(250 * time.Millisecond)
			m.setPant(pError)
			return m, nil
		}
		m.pdf, m.info = v.pdf, v.info
		m.fijarPapel("Letter") // siempre Carta; el aviso sale si el PDF es de otro tamaño
		if st, err := os.Stat(m.ruta); err == nil {
			m.tam = st.Size()
		}
		m.setPant(pMenu)
		return m, nil
	case msgEnviado:
		m.enviandoP2 = false
		if v.err != nil {
			orig := v.err.Error()
			msg := Mensaje{"No se pudo enviar el trabajo.", ""}
			if e, ok := v.err.(*ErrLP); ok {
				msg, orig = TraducirErrorLP(e.Stderr), e.Stderr
			}
			m.err, m.errOrig, m.volver, m.retryPasada = &msg, orig, pVista, 0
			if len(m.jobs) > 0 { // ya salió algo: nunca recomenzar desde la pasada 1
				m.retryPasada, m.volver = m.pasada, pVolteo
			}
			m.shake = time.Now().Add(250 * time.Millisecond)
			m.setPant(pError)
			return m, nil
		}
		m.job, m.desconocido, m.inicioJob = v.id, 0, time.Now()
		m.jobs = append(m.jobs, v.id)
		if m.usaDuplex() {
			s := m.sesion("PASADA" + fmt.Sprint(m.pasada) + "_IMPRIMIENDO")
			s.Job1 = m.job
			s.Guardar()
		}
		m.setPant(pImprimiendo)
		return m, cmdPoll(m.ctx, m.cola, v.id, m.dry, time.Second)
	case msgPoll:
		if v.job != m.job || m.pant != pImprimiendo {
			return m, nil
		}
		return m.avanzarJob(v)
	case tea.KeyMsg:
		return m.tecla(v)
	}
	return m, nil
}

// irABuscar decide qué hacer con el argumento inicial: abrir un archivo
// existente, listar una carpeta o usar el texto como búsqueda en la carpeta
// actual.
func (m *model) irABuscar() tea.Cmd {
	arg := ""
	if m.argRuta != "" {
		ruta := m.argRuta
		m.argRuta = "" // ya se usa; las recargas no deben repetirlo
		st, err := os.Stat(ruta)
		switch {
		case err == nil && !st.IsDir():
			return m.elegir(ruta)
		case err == nil:
			arg = ruta
		case os.IsNotExist(err) && !strings.ContainsRune(ruta, filepath.Separator):
			// No es una ruta: se busca por nombre en la carpeta actual.
			m.busq.SetValue(ruta)
		default:
			msg := Validar(ruta)
			m.err, m.volver = msg, pBuscador
			m.setPant(pError)
			return nil
		}
	}
	m.setPant(pBuscador)
	return cmdArchivos(m.ctx, arg)
}

func (m *model) elegir(ruta string) tea.Cmd {
	if msg := Validar(ruta); msg != nil {
		m.err, m.volver = msg, pBuscador
		m.shake = time.Now().Add(250 * time.Millisecond)
		m.setPant(pError)
		return nil
	}
	if abs, err := filepath.Abs(ruta); err == nil {
		ruta = abs
	}
	m.ruta = ruta
	m.analID++
	var ctx context.Context
	ctx, m.cancel = context.WithCancel(m.ctx)
	m.setPant(pAnalizando)
	return cmdAnalizar(ctx, m.analID, ruta, time.Duration(m.cfg.TimeoutSoffice)*time.Second)
}

// avanzarJob decide qué hacer cuando cambia el estado de un trabajo.
func (m *model) avanzarJob(v msgPoll) (tea.Model, tea.Cmd) {
	switch v.st {
	case TrabajoActivo:
		m.textoJob = "Imprimiendo"
		if time.Since(m.inicioJob) > 20*time.Second && (!v.cola.Habilitada) {
			m.textoJob = "Parece detenido: " + razonCola(v.cola)
		}
		return m, cmdPoll(m.ctx, m.cola, m.job, m.dry, time.Second)
	case TrabajoDesconocido:
		m.desconocido++
		if m.desconocido < 3 {
			return m, cmdPoll(m.ctx, m.cola, m.job, m.dry, time.Second)
		}
	}
	// Si CUPS dice que el trabajo se canceló o falló, NO se sigue: pasar a
	// «voltea las hojas» con una pasada 1 que no salió gastaría papel.
	if v.final == "canceled" || v.final == "aborted" {
		que := "El trabajo se canceló."
		if v.final == "aborted" {
			que = "El trabajo falló en la impresora."
		}
		msg := Mensaje{que, "Enter reintenta el envío"}
		m.err, m.errOrig, m.volver, m.retryPasada = &msg, "estado de CUPS: "+v.final, pVista, m.pasada
		m.shake = time.Now().Add(250 * time.Millisecond)
		m.setPant(pError)
		return m, nil
	}
	return m.siguiente()
}

func razonCola(c EstadoCola) string {
	if c.Razon != "" {
		return c.Razon
	}
	return "revisa papel, tinta y cable"
}

func (m *model) siguiente() (tea.Model, tea.Cmd) {
	if !m.usaDuplex() {
		m.setPant(pListo)
		return m, nil
	}
	if m.pasada == 1 {
		s := m.sesion("ESPERA_VOLTEO")
		s.Guardar()
		m.enviandoP2 = false
		m.setPant(pVolteo)
		return m, nil
	}
	if m.copia < m.copias {
		m.copia++
		m.pasada = 1
		return m, m.enviarPasada(1)
	}
	BorrarSesion()
	m.setPant(pListo)
	return m, nil
}

func (m *model) enviarPasada(n int) tea.Cmd {
	m.pasada = n
	m.textoJob = "Enviando"
	return cmdEnviar(m.ctx, m.argumentos(n), m.dry, m.ruta)
}

func (m *model) empezarImpresion() tea.Cmd {
	m.jobs, m.copia, m.pasada = nil, 1, 1
	if m.usaDuplex() {
		sel, _ := m.seleccion()
		m.plan = PlanificarDuplex(sel)
		m.sesion("PASADA1_ENVIADA").Guardar()
		if m.pdfPasada[1] == "" { // por si no se preparó al salir del menú
			if err := m.prepararDuplex(); err != nil {
				msg := Mensaje{"No pude preparar las páginas del doble cara.", "Instala qpdf: sudo pacman -S qpdf"}
				m.err, m.errOrig, m.volver = &msg, err.Error(), pVista
				m.setPant(pError)
				return nil
			}
		}
	}
	return m.enviarPasada(1)
}

func (m *model) salir(txt string) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.salida = txt
	m.fin, m.dejar = true, m.pant == pListo
	return m, tea.Quit
}

func (m *model) tecla(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		txt := ""
		switch m.pant {
		case pImprimiendo, pVolteo:
			txt = "Lo que ya se envió sigue en la cola de la impresora."
			if m.pant == pVolteo {
				txt += " Para continuar el doble cara: print --reanudar"
			}
		}
		return m.salir(txt)
	}
	switch m.pant {
	case pReanudar:
		switch s {
		case "enter":
			return m.reanudar()
		case "d", "esc":
			BorrarSesion()
			m.ses = nil
			return m, m.irABuscar()
		case "q":
			return m.salir("")
		}
	case pBuscador:
		return m.teclaBuscador(k)
	case pAnalizando:
		if s == "esc" {
			m.cancel()
			m.analID++
			m.setPant(pBuscador)
		}
	case pMenu:
		return m.teclaMenu(k)
	case pVista:
		return m.teclaVista(k)
	case pVolteo:
		return m.teclaVolteo(k)
	case pListo:
		switch s {
		case "enter", "q", "esc":
			return m.salir("")
		case "n":
			m.setPant(pMenu)
		}
	case pImprimiendo:
		if s == "c" && m.job != "" && m.job != "dry-run" {
			return m, cmdCancelarUno(m.ctx, m.job)
		}
		if s == "q" {
			return m.salir("Lo que ya se envió sigue en la cola de la impresora.")
		}
	case pError:
		if m.retryPasada > 0 && (s == "enter" || s == "r" || s == "esc") {
			rp := m.retryPasada
			m.retryPasada = 0
			if s == "esc" {
				if rp == 2 {
					m.setPant(pVolteo)
					return m, nil
				}
				return m.salir("Doble cara interrumpido. Reanuda con: print --reanudar")
			}
			return m, m.enviarPasada(rp)
		}
		switch s {
		case "enter", "r", "esc":
			if m.volver == pArranque {
				m.setPant(pArranque)
				return m, cmdPreflight(m.ctx, m.cola, false)
			}
			if m.volver == pBuscador && m.ruta != "" && m.err != nil && strings.Contains(m.err.Accion, "reintentar") {
				return m, m.elegir(m.ruta)
			}
			m.setPant(m.volver)
			if m.volver == pBuscador && len(m.archivos) == 0 {
				return m, cmdArchivos(m.ctx, "")
			}
		case "c":
			if m.err != nil && strings.HasPrefix(m.err.Accion, "sudo ") {
				m.msgCopia = "copiado"
				if !copiarPortapapeles(m.err.Accion) {
					m.msgCopia = "no hay wl-copy ni xclip"
				}
			}
		case "q":
			return m.salir("")
		}
	}
	return m, nil
}

func (m *model) reanudar() (tea.Model, tea.Cmd) {
	s := m.ses
	m.ruta, m.pdf, m.info = s.Archivo, s.PDF, InfoPDF{Paginas: s.Paginas}
	m.cola = s.Opciones["cola"]
	if indiceDe(m.todosPapeles, s.Opciones["papel"]) >= 0 {
		m.fijarPapel(s.Opciones["papel"])
	}
	for i, t := range tintas {
		if t.Val == s.Opciones["tinta"] {
			m.iTinta = i
		}
	}
	for i, o := range orients {
		if o.Val == s.Opciones["orient"] {
			m.iOrient = i
		}
	}
	for i, o := range medios {
		if o.Val == s.Opciones["medio"] {
			m.iMedio = i
		}
	}
	m.escalaForzada, m.pdfImp = s.Opciones["escala"], s.PDF
	m.bordes = s.Opciones["bordes"] == "1"
	m.revInvertir = s.Opciones["revinv"] == "1"
	m.extra = strings.Fields(s.Opciones["extra"])
	m.caras, m.copias, m.copia, m.pasada = 1, max(1, s.Copias), max(1, s.Copia), 1
	m.modoRango = true
	m.rangoTxt = FormatoRangos(mezclar(s.A, s.R))
	m.plan = PlanificarDuplex(mezclar(s.A, s.R))
	m.jobs = []string{s.Job1}
	if _, err := os.Stat(m.pdf); err != nil {
		msg := Mensaje{"El PDF de la sesión ya no existe.", "Empieza de nuevo desde el buscador"}
		BorrarSesion()
		m.err, m.volver = &msg, pBuscador
		m.setPant(pError)
		return m, nil
	}
	m.enviandoP2 = false
	if err := m.prepararDuplex(); err != nil {
		msg := Mensaje{"No pude preparar las páginas del doble cara.", "Instala qpdf: sudo pacman -S qpdf"}
		m.err, m.errOrig, m.volver = &msg, err.Error(), pBuscador
		m.setPant(pError)
		return m, nil
	}
	m.setPant(pVolteo)
	return m, nil
}

func mezclar(a, r []int) []int {
	var out []int
	for i := range a {
		out = append(out, a[i])
		if i < len(r) {
			out = append(out, r[i])
		}
	}
	return out
}

func (m *model) teclaBuscador(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		return m.salir("")
	case "up", "ctrl+p":
		if m.cur > 0 {
			m.cur--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.cur < len(m.coin)-1 {
			m.cur++
		}
		return m, nil
	case "enter":
		if m.cur < len(m.coin) {
			return m, m.elegir(m.coin[m.cur].Ruta)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.busq, cmd = m.busq.Update(k)
	m.coin = Buscar(m.archivos, m.busq.Value())
	m.cur = 0
	return m, cmd
}

func (m *model) teclaMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s != "x" {
		m.confirmCancel = false
	}
	m.nota = ""
	if m.escalaAbierta {
		switch s {
		case "enter", " ":
			m.fijarEscala(m.escalaPick)
			m.escalaAbierta = false
			return m, nil
		case "left", "h", "right", "l":
			// sigue al manejo normal de cambiar()
		default:
			m.escalaAbierta = false
			m.escalaCur = max(0, posEn(m.escalasVisibles(), m.iEscala))
			if s == "esc" {
				return m, nil
			}
		}
	}
	if m.papelAbierto {
		switch s {
		case "enter", " ":
			m.fijarPapel(m.todosPapeles[m.papelPick])
			m.papelAbierto = false
			return m, nil
		case "left", "h", "right", "l":
			// sigue al manejo normal de cambiar()
		default:
			m.papelAbierto = false
			m.papelCur = max(0, indiceDe(m.papelVis, m.papel))
			if s == "esc" {
				return m, nil
			}
		}
	}
	// edición directa del rango cuando la fila Páginas está en modo rango
	if m.fila == rPaginas && m.modoRango {
		if len(k.Runes) == 1 && strings.ContainsRune("0123456789,- ", k.Runes[0]) {
			if len(m.rangoTxt) < 40 {
				m.rangoTxt += string(k.Runes)
			}
			return m, nil
		}
		if s == "backspace" && m.rangoTxt != "" {
			m.rangoTxt = m.rangoTxt[:len(m.rangoTxt)-1]
			return m, nil
		}
	}
	switch s {
	case "q", "esc":
		if s == "esc" {
			m.setPant(pBuscador)
			return m, cmdArchivos(m.ctx, "")
		}
		return m.salir("")
	case "up", "k":
		for {
			m.fila = (m.fila + nFilas - 1) % nFilas
			if m.visible(m.fila) {
				break
			}
		}
	case "down", "j", "tab":
		for {
			m.fila = (m.fila + 1) % nFilas
			if m.visible(m.fila) {
				break
			}
		}
	case "left", "h":
		m.cambiar(-1)
	case "right", "l":
		m.cambiar(1)
	case "+":
		if m.fila == rCopias {
			m.cambiar(1)
		}
	case "-":
		if m.fila == rCopias {
			m.cambiar(-1)
		}
	case "d":
		m.descartado = true
	case "x": // cancelar los trabajos propios pendientes (dos pulsaciones)
		if m.propiosEnCola() == 0 {
			return m, nil
		}
		if !m.confirmCancel {
			m.confirmCancel = true
			return m, nil
		}
		m.confirmCancel = false
		return m, cmdCancelarTodos(m.ctx, m.cola, false)
	case "e": // habilitar la cola detenida (pide la contraseña en esta terminal)
		if m.colaDetenida() {
			return m, cmdHabilitar(m.cola)
		}
	case "r":
		m.recargando = true
		m.setPant(pArranque)
		return m, cmdPreflight(m.ctx, m.cola, false)
	case "enter", " ":
		if p := m.problemaImprimir(); p != "" {
			m.shake = time.Now().Add(250 * time.Millisecond)
			return m, nil
		}
		m.pdfImp, m.verOrden = "", false
		if m.hayDesajuste() {
			var p string
			var err error
			switch escalas[m.iEscala].Val {
			case "real":
				p, err = PrepararReal(m.ctx, m.pdf, m.info.Ancho, m.info.Alto, m.papel)
			case "estirar":
				p, err = PrepararEstirar(m.ctx, m.pdf, m.info.Ancho, m.info.Alto, m.papel)
			}
			if err != nil {
				msg := Mensaje{"No pude preparar la escala elegida.", "Elige Ajustar o Cubrir"}
				m.err, m.errOrig, m.volver = &msg, err.Error(), pMenu
				m.setPant(pError)
				return m, nil
			}
			m.pdfImp = p
		}
		if err := m.prepararDuplex(); err != nil {
			msg := Mensaje{"No pude preparar las páginas del doble cara.", "Instala qpdf: sudo pacman -S qpdf"}
			m.err, m.errOrig, m.volver = &msg, err.Error(), pMenu
			m.setPant(pError)
			return m, nil
		}
		m.editando, m.editErr = false, ""
		m.setPant(pVista)
	}
	return m, nil
}

func ciclo(i, d, n int) int { return (i + d + n) % n }

func (m *model) cambiar(d int) {
	m.pulso = time.Now().Add(180 * time.Millisecond)
	switch m.fila {
	case rTinta:
		m.iTinta = ciclo(m.iTinta, d, len(tintas))
		if tintas[m.iTinta].Val == "color" {
			m.avisoCMY = true
		}
	case rCalidad:
		m.iCalidad = ciclo(m.iCalidad, d, len(calidades))
	case rPapel:
		if m.papelAbierto {
			m.papelPick = min(len(m.todosPapeles)-1, max(0, m.papelPick+d))
			break
		}
		m.papelCur = min(len(m.papelVis), max(0, m.papelCur+d)) // sin dar la vuelta
		if m.papelCur == len(m.papelVis) {                      // "Más"
			m.papelAbierto = true
			m.papelPick = max(0, indiceDe(m.todosPapeles, m.papel))
		} else {
			m.papel = m.papelVis[m.papelCur]
		}
	case rEscala:
		if m.escalaAbierta {
			m.escalaPick = min(len(escalas)-1, max(0, m.escalaPick+d))
			break
		}
		vis := m.escalasVisibles()
		m.escalaCur = min(len(vis), max(0, m.escalaCur+d)) // sin dar la vuelta
		if m.escalaCur == len(vis) {                       // «Más»
			m.escalaAbierta = true
			m.escalaPick = m.iEscala
		} else {
			m.iEscala = vis[m.escalaCur]
		}
	case rCaras:
		m.caras = ciclo(m.caras, d, 2)
	case rPaginas:
		m.modoRango = !m.modoRango
	case rCopias:
		m.copias = min(99, max(1, m.copias+d))
	case rOrient:
		m.iOrient = ciclo(m.iOrient, d, len(orients))
	case rMedio:
		m.iMedio = ciclo(m.iMedio, d, len(medios))
	case rBordes:
		m.bordes = !m.bordes
	}
}

func (m *model) teclaVista(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editando {
		switch k.String() {
		case "esc":
			m.editando = false
			return m, nil
		case "enter":
			var ok []string
			perm := clavesPermitidas()
			for _, kv := range strings.Fields(m.edit.Value()) {
				c, _, tiene := strings.Cut(kv, "=")
				if !tiene || !perm[c] {
					m.editErr = fmt.Sprintf("opción no permitida: %q (permitidas: %s)", kv, claves())
					return m, nil
				}
				_, val, _ := strings.Cut(kv, "=")
				if vs := ValoresPPD(m.cola, c); len(vs) > 0 && indiceDe(vs, val) < 0 {
					m.editErr = fmt.Sprintf("valor no válido para %s (válidos: %s)", c, strings.Join(vs, " "))
					return m, nil
				}
				ok = append(ok, kv)
			}
			m.extra, m.editErr, m.editando = ok, "", false
			return m, nil
		}
		var cmd tea.Cmd
		m.edit, cmd = m.edit.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "esc":
		m.dupAviso, m.dupOK = "", false
		m.setPant(pMenu)
	case "e":
		m.editando = true
		m.edit.SetValue(strings.Join(m.extra, " "))
		return m, m.edit.Focus()
	case "o":
		m.verOrden = !m.verOrden
	case "y":
		m.msgCopia = "orden copiada"
		if !copiarPortapapeles("lp " + shellMostrar(m.argumentos(1))) {
			m.msgCopia = "no hay wl-copy ni xclip"
		}
	case "enter":
		if !m.dupOK {
			if aviso := m.avisoDuplicado(); aviso != "" {
				m.dupAviso, m.dupOK = aviso, true
				m.shake = time.Now().Add(250 * time.Millisecond)
				return m, nil
			}
		}
		m.dupAviso, m.dupOK = "", false
		return m, m.empezarImpresion()
	}
	return m, nil
}

func (m *model) teclaVolteo(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter": // un Enter y listo; el segundo no envía otra vez
		if m.enviandoP2 {
			return m, nil
		}
		m.enviandoP2 = true
		return m, m.enviarPasada(2)
	case "g": // girar 180° la tanda que se gira (para calibrar con papel real)
		if !m.enviandoP2 {
			m.revInvertir = !m.revInvertir
			m.sesion("ESPERA_VOLTEO").Guardar()
		}
	case "q":
		return m.salir("Doble cara pendiente. Para continuar: print --reanudar")
	}
	return m, nil
}

// shellMostrar solo para mostrar/copiar; nunca se ejecuta con shell.
func shellMostrar(args []string) string {
	var out []string
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"'$`;&|<>()*?[]{}!#~\\") || a == "" {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		out = append(out, a)
	}
	return strings.Join(out, " ")
}

func (m *model) propiosEnCola() int {
	n := 0
	for _, t := range m.trabajosCola {
		if t.Propio {
			n++
		}
	}
	return n
}

// colaDetenida: la cola está parada pero el USB sí se ve (se puede habilitar).
func (m *model) colaDetenida() bool {
	usb := true
	parada := false
	for _, h := range m.hallazgos {
		switch h.ID {
		case "P-6":
			usb = false
		case "P-4", "P-5":
			parada = true
		}
	}
	return parada && usb
}

// avisoDuplicado avisa si este mismo archivo ya está esperando en la cola.
func (m *model) avisoDuplicado() string {
	base := filepath.Base(m.ruta)
	for _, t := range m.trabajosCola {
		if t.Origen == "print" && filepath.Base(t.Nombre) == base && t.Estado != "imprimiendo" {
			return fmt.Sprintf("«%s» ya está esperando en la cola (%s). Enter otra vez para enviarlo de nuevo.", base, edadHumana(t.EdadS))
		}
	}
	return ""
}
