# Documentación completa de CLI-Print

Menú de impresión para Linux. Esta guía cuenta todo: la configuración de
referencia, cómo se usa, cómo está hecho por dentro, lo que se probó con papel y
lo que conviene saber si algo falla.

> **Nota:** los nombres de cola, modelos y valores concretos que aparecen aquí son
> el ejemplo con el que se desarrolló y probó el programa. `print` funciona con
> cualquier cola de CUPS; lo único obligatorio es que exista una.

Versión del programa: **0.3.0** · Probado: **2026-10-17** · Idioma: español.

---

## Índice
1. [Qué es y para qué sirve](#1-qué-es-y-para-qué-sirve)
2. [La impresora](#2-la-impresora)
3. [Instalación](#3-instalación)
4. [Uso diario](#4-uso-diario)
5. [Todas las opciones del menú](#5-todas-las-opciones-del-menú)
6. [El doble cara manual](#6-el-doble-cara-manual)
7. [La escala (A4 en hojas Carta)](#7-la-escala-a4-en-hojas-carta)
8. [Línea de comandos](#8-línea-de-comandos)
9. [Cómo está hecho por dentro](#9-cómo-está-hecho-por-dentro)
10. [Dónde guarda las cosas](#10-dónde-guarda-las-cosas)
11. [Configuración](#11-configuración)
12. [Pruebas](#12-pruebas)
13. [Lo que se verificó con papel](#13-lo-que-se-verificó-con-papel)
14. [Si algo falla](#14-si-algo-falla)
15. [Límites conocidos](#15-límites-conocidos)
16. [Decisiones de diseño y por qué](#16-decisiones-de-diseño-y-por-qué)
17. [Pendiente e ideas](#17-pendiente-e-ideas)
18. [Historial de lo aprendido](#18-historial-de-lo-aprendido)

---

## 1. Qué es y para qué sirve

`print` es un programa de terminal para imprimir documentos sin recordar la
sintaxis de `lp`. Eliges el archivo, decides cómo imprimirlo y confirmas.

Resuelve cuatro problemas habituales:

- **No hay doble cara automático.** `print` guía el doble cara manual y calcula el
  orden de las páginas para que, al terminar, la página 1 quede arriba.
- **Modo de color configurable.** Se puede imprimir solo con tintas de color,
  con negro y color, o en blanco y negro.
- **El documento y el papel no coinciden.** Ofrece cuatro formas de adaptarlos.
- **La cola de CUPS se detiene** si mandas algo con la impresora apagada. `print` lo
  avisa y permite recuperarse.

Es un único binario (Go + Bubble Tea) con el menú interactivo y los subcomandos.

Todo se hace **sin sudo**, salvo habilitar una cola detenida, que pide la contraseña
a través de `sudo` (en la terminal) o `pkexec` (ventana gráfica). `print` nunca ve la
contraseña.

---

## 2. Entorno de referencia

Lo que sigue es el ejemplo con el que se desarrolló el programa. Ningún valor es
obligatorio: `print` solo necesita una cola de CUPS.

### Datos
| Dato | Valor |
|---|---|
| Modelo | Canon PIXMA G2110 |
| Se identifica por USB como | `G2010 series` (familia G2000) |
| ID USB | `04a9:183a` |
| Número de serie | `05DBAE` |
| URI del dispositivo | `usb://Canon/G2010%20series?serial=05DBAE&interface=1` |
| Cola de CUPS | `Canon_G2110` (predeterminada) |
| Controlador | Gutenprint 5.3.5 — `stp-bjc-G2000-series.5.3.sim.ppd.gz` |
| Valores por defecto de la cola | `PageSize=Letter`, `MediaType=Plain`, `ColorModel=RGB` |
| Papel disponible | solo **Carta** |
| Entrada / salida del papel | bandeja trasera / sale por delante, **boca arriba** |

### Restricciones del ejemplo
- **Sin duplexor.** El doble cara es manual, en dos pasadas.
- **Modo «Solo color».** `ColorModel=CMY` forma el negro mezclando cian, magenta y
  amarillo; puede salir pardo, no negro puro.
- **Sin niveles de tinta.** Si CUPS no informa `marker-levels`, no se muestra barra.
- Algunos modelos tienen depósitos transparentes: se ve la tinta a simple vista.

### Nota sobre `usblp`
En algunas instalaciones el módulo `usblp` del kernel choca con CUPS
(`Failed to re-attach "usblp" kernel module`). Si aparece, desactívalo con una
entrada `blacklist usblp` en `/etc/modprobe.d/`.

### Qué opciones del controlador funcionan de verdad
Muchas opciones del panel de CUPS son decorativas. Se comprobó una por una pasando el
mismo PDF por el filtro final `rastertogutenprint.5.3` y comparando el resultado
(comparar solo el raster no basta: el filtro final consume opciones que el raster no
refleja). El resultado está en `opciones_verificadas.json`:

| Opción | Estado | Notas |
|---|---|---|
| `ColorModel` (`CMY`, `RGB`, `Gray`…) | real | cambia el raster |
| `PageSize`, `StpOrientation` | real | |
| `MediaType`, `StpFullBleed`, `StpiShrinkOutput` | real | el efecto físico se confirma en papel |
| `StpImageType`, `StpBrightness`, `StpContrast`, `StpSaturation` | real | no se ofrecen en el menú |
| `StpInkType` | real, sin confirmar | `Gray` podría usar solo negra: no se ofrece |
| `StpQuality`, `StpResolution`, `StpColorCorrection` | **ignoradas** | no se ofrecen |
| `page-ranges`, `copies` | real | opciones de `lp` |

El menú solo ofrece opciones verificadas.

---

## 3. Instalación

### Requisitos
| Programa | Para qué |
|---|---|
| CUPS (`lp`, `lpstat`, `cancel`, `ipptool`) | imprimir y consultar la cola |
| LibreOffice (`soffice`) | convertir Word, ODT, hojas de cálculo… a PDF |
| Poppler (`pdfinfo`, `pdftotext`) | leer páginas y tamaño de un PDF |
| `qpdf` | reordenar páginas y validar PDF |
| Ghostscript (`gs`) | preparar *Tamaño real* y *Estirar* |
| Go ≥ 1.27 | compilar |

Fuentes de Office: `sudo pacman -S ttf-carlito ttf-caladea` (si no, un `.docx` con
Calibri o Cambria se recoloca y puede cambiar de número de páginas).

### Compilar e instalar
```bash
git clone https://github.com/Zobercito/cli-print.git
cd cli-print
make build
make install        # ~/.local/bin/print
```

O sin `make`:
```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ~/.local/bin/print .
```

### Desinstalar
```bash
make uninstall
print --limpiar     # borra caché, estado y configuración (opcional)
```

---

## 4. Uso diario

```bash
print                          # lista los documentos de la carpeta actual
print informe.pdf              # va directo al menú de ese archivo
print docu                     # filtra por nombre en la carpeta actual
print ~/Descargas/             # lista los documentos de esa carpeta
print "mi informe (v2).docx"   # nombres con espacios, tildes o comillas: sin problema
print informe.pdf --dry-run    # enseña lo que haría y no imprime
```

El buscador muestra solo los archivos de **esa carpeta**: no recorre subcarpetas ni
busca en Descargas, Documentos o Escritorio. Para imprimir algo de otro sitio, `cd`
a esa carpeta y ejecuta `print` allí.

Si el argumento no es un archivo ni una carpeta existente, se usa como **texto de
búsqueda** en la carpeta actual: `print docu` abre el buscador ya filtrado por «docu».
Es la búsqueda difusa de siempre (sin distinguir mayúsculas ni tildes).

El menú se dibuja **en la misma terminal**, debajo de lo que ya tenías, y al salir
desaparece (la pantalla «Listo» se queda). Con `--pantalla-completa` usa la pantalla
completa. Necesita una terminal de al menos 60×24.

### Flujo de pantallas
```
arranque ─► (¿doble cara pendiente? → reanudar) ─► buscador ─► analizando ─► menú
   ─► vista previa ─► imprimiendo ─► [volteo] ─► imprimiendo ─► listo
```

1. **Arranque:** revisa CUPS, la cola, la conexión USB, LibreOffice, `pdfinfo` y las
   fuentes. Los problemas salen como una línea de aviso en el menú.
2. **Buscador:** busca mientras escribes, sin distinguir mayúsculas ni tildes. Solo
   muestra documentos (nunca imágenes) de la carpeta actual.
3. **Analizando:** convierte a PDF si hace falta y lee páginas y tamaño de papel.
4. **Menú:** las opciones (sección 5), con un resumen de lo que va a pasar.
5. **Vista previa:** lo que se va a imprimir, en palabras. `o` muestra la orden `lp`.
6. **Imprimiendo:** sigue el trabajo en CUPS. `c` lo cancela.
7. **Volteo** (solo doble cara): animación de cómo girar el bloque. `Enter` envía la
   pasada 2.
8. **Listo:** resumen de lo impreso (archivo, páginas, hojas, papel, tinta).

### Teclas
| Pantalla | Teclas |
|---|---|
| Buscador | escribir · `↑↓` · `Enter` elegir · `Esc` salir |
| Menú | `↑↓` mover · `←→` cambiar · `Enter` o espacio → vista previa · `r` recargar · `e` habilitar la cola · `x` cancelar trabajos en cola (dos pulsaciones) · `d` ocultar avisos · `Esc` atrás · `q` salir |
| Páginas (modo Rango) | escribir `1-3,7,10-`; borrar con retroceso |
| Papel / Escala | `→` hasta **Más** abre el selector: `←→` elegir · `Enter` confirmar · `Esc` cancelar |
| Vista previa | `Enter` **ENVIAR** · `o` ver la orden · `e` editar opciones extra · `y` copiar la orden · `Esc` volver |
| Imprimiendo | `c` cancelar el trabajo · `q` salir (lo enviado sigue en la cola) |
| Volteo | `Enter` imprimir la pasada 2 · `g` girar el reverso 180° · `q` guardar y salir |
| Error | `Enter` reintentar · `c` copiar el comando sugerido · `q` salir |
| Reanudar | `Enter` reanudar · `d` descartar · `q` salir |

`Ctrl+C` sale limpiamente desde cualquier pantalla. Si ya había algo en la cola de la
impresora, lo dice: sigue ahí.

---

## 5. Todas las opciones del menú

La cabecera dice siempre «Impresora». El nombre real, el que declara CUPS, aparece
en el resumen final.

| Fila | Valores | Qué hace |
|---|---|---|
| **Tinta** | Normal · B/N · Solo color | `RGB` (por defecto) · `Gray` · `CMY` |
| **Papel** | Carta · A4 · *Más* | tamaños reales del controlador (Carta, A4, Legal, Executive, Statement, Postcard, 4×6…) |
| **Escala** | Ajustar · Tamaño real · *Más* (Estirar, Cubrir) | solo aparece si el PDF y el papel no coinciden (sección 7) |
| **Caras** | Una · Doble (manual) | sección 6 |
| **Páginas** | Todas · Rango | `1-3`, `1,5,9`, `1-3,7,10-`; se valida en vivo |
| **Copias** | 1–99 | con doble cara, un ciclo completo por copia |
| **Orientación** | Vertical · Horizontal | `StpOrientation` |
| **Tipo papel** | Normal · Mate · Brillante | cantidad de tinta y perfil de color; con papel común, déjalo en Normal |
| **Sin bordes** | No · Sí | imprime hasta el filo de la hoja (fotos, tarjetas) |

Debajo hay una línea de resumen («4 páginas × 1 copia, Carta vertical, solo color») y,
si algo impide imprimir, el motivo en rojo. Los avisos de arriba son pocos y cortos:
impresora desconectada o cola detenida (con `e` para habilitarla), trabajos esperando
en la cola (con `x` para cancelarlos) y fuentes de Office ausentes.

**«Solo color»** forma el negro mezclando cian, magenta y amarillo: puede verse
pardo. **«Normal»** usa el cartucho negro y ahorra tinta de color.

**«B/N»** usa solo la tinta negra.

---

## 6. El doble cara manual

### Cómo se imprime
La impresora saca las hojas **boca arriba y la última queda encima**. Para que al final
la página 1 quede arriba, con la 2 detrás, luego la 3 y su 4, se hace en este orden:

| | Qué imprime | Orden |
|---|---|---|
| **Pasada 1** | las caras de atrás | de la última hoja a la primera |
| **Pasada 2** | las caras de adelante | de la última hoja a la primera |

Con 4 páginas: pasada 1 = **4, 2**; pasada 2 = **3, 1**.

### Qué hace el usuario
1. Enviar. Salen las hojas con las caras de atrás.
2. Cuando termina, gira el bloque **por un borde largo** (los de los lados), a la
   derecha o a la izquierda, como la página de un libro, y lo vuelve a poner en la
   bandeja trasera, igual que las hojas en blanco.
3. `Enter`. Salen las caras de adelante.

La pantalla de volteo lo enseña con una animación: la hoja se encoge hacia una
bisagra y pasa al otro lado. Un borde de color queda fijo y el opuesto cambia de lado.

### Cómo se calcula
Las hojas se forman **por posición** en las páginas elegidas, no por paridad: en la
selección `2-7` las hojas son (2,3), (4,5), (6,7), y los frentes (2, 4, 6) son pares.
Funciona con cualquier rango.

CUPS solo sabe imprimir rangos en orden **ascendente** (`outputorder=reverse` no lo
aplica `pdftopdf` aquí), así que `print` crea con `qpdf` un PDF reordenado por pasada
y lo envía entero. Los archivos van a la caché con un nombre que incluye el contenido.

### Páginas impares
La última hoja no tiene reverso, pero tiene que estar en el bloque desde la pasada 1
para ocupar su sitio. `print` añade **una página en blanco al principio de la pasada
1** (la impresora pasa una hoja sin tinta) y la pasada 2 empieza por esa página sola.
Con 5 páginas: pasada 1 = *hoja en blanco, 4, 2*; pasada 2 = *5, 3, 1*.
Solo si faltara Ghostscript, la pantalla de volteo pide añadir la hoja a mano.

### Orientación
Con el giro por un borde largo, **las dos tandas van con la misma orientación**
(`Portrait`). Con contenido horizontal la regla se invierte. `g` en la pantalla de
volteo invierte la orientación de la tanda de reversos por si hiciera falta calibrar
con otro papel. En la configuración, `giro = "tortita"` cambia al giro por un borde
corto (como la hoja de un calendario).

### Interrupciones
- Si sales entre las dos pasadas (`q`, `Ctrl+C` o cierras la terminal), la sesión queda
  guardada (`sesion.json`). La próxima vez `print` ofrece **reanudar**.
- Si CUPS dice que la pasada 1 se **canceló o falló**, `print` **no** avanza al volteo
  (gastaría papel). Muestra el error y `Enter` reintenta esa pasada.
- Si falla el envío de la pasada 2, `Enter` la reintenta y `Esc` vuelve al volteo; nunca
  recomienza desde la pasada 1.
- Una sesión se descarta si pasan más de 24 h o el archivo cambió.

---

## 7. La escala (A4 en hojas Carta)

La fila **Escala** aparece cuando el PDF y el papel no coinciden, o cuando el PDF es
apaisado y la hoja vertical. Un A4 mide 29,7 cm de largo; una hoja Carta, 27,9 cm.

| Opción | Franjas blancas | Corta algo | Deforma | Cómo se hace |
|---|---|---|---|---|
| **Ajustar** (por defecto) | sí, a los lados | no | no | CUPS: `print-scaling=fit` |
| **Tamaño real** | no | sí, por abajo (~1,8 cm) | no | `print` prepara un PDF 1:1, borde de arriba pegado y centrado |
| **Estirar** | no | no | sí, un poco (~3 % de ancho, ~6 % de alto) | `print` escala ancho y alto por separado con Ghostscript |
| **Cubrir** | no | sí, arriba y abajo a partes iguales | no | CUPS: `print-scaling=fill` |

*Ajustar* y *Tamaño real* están a la vista; *Estirar* y *Cubrir* están en **Más**.

### PDF apaisado
En papel vertical, CUPS lo gira en sentido horario y **siempre lo reduce**, diga lo que
diga `print-scaling`. Por eso *Tamaño real* y *Estirar* los prepara `print`: giran el
documento en el mismo sentido que CUPS, para que las tres opciones salgan acostadas
hacia el mismo lado.

### Detalles técnicos que costaron aprender
- Ghostscript añade `/Rotate` a la página según el texto y **deshace el giro**: se usa
  siempre `-dAutoRotatePages=/None`.
- Los PDF preparados llevan una **versión en el nombre** (`real_Letter_v3.pdf`): si
  cambia la forma de prepararlos, los archivos viejos de la caché no se reutilizan.

---

## 8. Línea de comandos

### Opciones
| Opción | Qué hace |
|---|---|
| `--dry-run` | muestra la orden `lp` y no imprime |
| `--sin-animacion` | desactiva las animaciones |
| `--ascii` | fuerza bordes y símbolos ASCII |
| `--pantalla-completa` | usa la pantalla completa en vez de dibujar en la terminal |
| `--reanudar` | comprueba que haya un doble cara pendiente |
| `--limpiar` | borra caché, estado y configuración |
| `--version` | versión |
| `--copias N`, `--paginas R`, `--papel P`, `--tinta T` | solo sin terminal (scripts) |

Las opciones valen **antes o después** del archivo.

### Sin terminal (scripts)
```bash
print --copias 2 --paginas 1-3 --papel A4 --tinta normal informe.pdf
```
Usa Carta por defecto y *Ajustar* si el documento no coincide. No admite doble cara
(exige girar las hojas a mano).

### Subcomandos
Todos admiten `--json`; los que actúan sobre una cola, `--cola NOMBRE`. Son la base
para integrar `print` en scripts o en cualquier interfaz externa.

| Comando | Qué hace |
|---|---|
| `print estado` | estado de la impresora y trabajos en cola |
| `print prueba` | envía la página de prueba de CUPS a la cola |
| `print detalles` | URI, modelo, ubicación, estado y opciones de la cola |
| `print nombres '<json>'` | guarda alias locales de las colas (no toca CUPS) |
| `print cancelar <id>` · `--todos` | cancela trabajos propios (sin sudo) |
| `print retener <id>` | deja un trabajo en espera |
| `print liberar <id>` | lo reanuda |
| `print imprimir <id>` · `--todos` | libera lo retenido y habilita la cola; con `<id>` deja los demás en espera |
| `print habilitar` | habilita la cola (`sudo` en terminal, `pkexec` sin ella) |

Códigos de salida: `0` bien · `1` la acción falló · `2` uso incorrecto.

### `print estado --json`
```json
{
  "v": 1, "ts": "2026-10-17T18:00:00-05:00", "cola": "Canon_G2110",
  "estado": "lista", "usb": true, "habilitada": true, "aceptando": true, "razon": "",
  "trabajos": [
    { "id": "Canon_G2110-25", "nombre": "informe.pdf", "usuario": "usuario",
      "bytes": 26624, "creado": "…", "edad_s": 180, "estado": "pendiente",
      "origen": "print", "propio": true }
  ],
  "alertas": [ { "nivel": "aviso", "codigo": "J-1", "texto": "…" } ],
  "tinta": null
}
```
`estado` es uno de: `sin_cups` · `sin_cola` · `desconectada` · `detenida` · `lista` ·
`imprimiendo` · `espera` · `error`. `estado` de un trabajo: `pendiente` · `retenido` ·
`imprimiendo` · `detenido`. `tinta` es siempre `null` (CUPS no la informa).

`print detalles --json` añade la ficha de la cola: `uri`, `modelo`, `ubicacion`,
`cups`, `predeterminada`, `alertas` y `opciones` (`PageSize`, `ColorModel`, `Duplex`,
`print-quality`…).

Tiempos máximos: consultar 8 s · detalles 10 s · prueba 30 s · cancelar y retener
20 s · habilitar e imprimir 3 min (da tiempo a escribir la contraseña).

---

## 9. Cómo está hecho por dentro

### Arquitectura
```
   udev (USB)                CUPS (cupsd)
        │                         │
        ▼                         ▼
 ┌────────────────────────────────────────────┐
 │ print (Go)                                 │
 │ menú · estado · cancelar · retener · imprimir │
 └────────────────────────────────────────────┘
```
El menú y los subcomandos comparten la misma lógica y muestran lo mismo.

### Archivos
| Archivo | Para qué |
|---|---|
| `main.go`, `cli.go`, `doc.go` | argumentos, subcomandos, modo sin terminal y documentación |
| `ui.go`, `update.go`, `vistas.go`, `tema.go` | la interfaz (Bubble Tea + Lip Gloss) |
| `cups.go`, `estado.go`, `acciones.go`, `preflight.go`, `detalles.go`, `nombres.go` | hablar con CUPS |
| `construir.go`, `rangos.go`, `duplex.go`, `duplexpdf.go`, `escala.go` | lógica de impresión |
| `convertir.go`, `analizar.go`, `descubrir.go`, `papel.go` | convertir, leer, buscar y tamaños de papel |
| `sesion.go`, `registro.go`, `config.go`, `ejecutar.go`, `errores.go` | estado en disco y utilidades |
| `ipp_jobs.test`, `opciones_verificadas.json` | consulta IPP embebida y opciones verificadas |
| `pruebas-papel/` | los PDF de las pruebas con papel real |
| `testdata/bin/` | CUPS falso para las pruebas (`lp`, `lpstat`, `ipptool`, `cancel`, `soffice`…) |

### Principios que se respetan
1. **Nunca construir una orden como texto para una shell.** `lp` y los demás se lanzan
   con argumentos separados y `--` antes del archivo. Los identificadores de trabajo y
   la cola se validan con expresiones regulares.
2. **Nunca fallar en silencio.** Todo error sale con su causa y, si la hay, la acción.
3. **No gastar tinta ni papel por una suposición.** Si CUPS dice que una pasada falló,
   no se sigue.
4. **Degradar con gracia.** Sin color, sin UTF-8 o sin animación sigue siendo usable.
5. **Sin sudo ni tocar el sistema.** Lo que pide permisos usa `sudo` o `pkexec` y
   muestra el comando exacto.
6. **Lo desconocido se muestra como desconocido** (no hay barra de tinta inventada).

### Cómo habla con CUPS
- `lpstat` con `LC_ALL=C` para poder interpretar su salida (el sistema está en español).
- `lp` para enviar; el identificador del trabajo se saca de «request id is …».
- `ipptool` con una consulta `Get-Jobs` embebida para leer nombre, dueño y estado de
  cada trabajo. **Hay que enviar `requesting-user-name`**: sin él CUPS oculta el nombre
  y el dueño, y los trabajos propios parecerían ajenos.
- Todas las llamadas externas llevan tiempo máximo y se lanzan en su propio grupo de
  procesos, para poder matar también a los hijos.
- La impresora se detecta por el **`device-uri` de la cola**: si es `usb://`, busca ese
  número de serie (o fabricante y modelo) en `/sys/bus/usb/devices`; si es de red, no
  puede saberlo y la da por conectada. Solo si no puede leer el URI usa el ID USB de la
  G2110.

### Conversión a PDF
Todo termina siendo un PDF. Los PDF se usan tal cual; el resto se convierte con
`soffice --headless` con un **perfil aislado** y tiempo máximo (60 s). Un candado de
archivo (`flock`) evita que dos instancias de `print` usen el mismo perfil a la vez.
La caché va por ruta + fecha + tamaño del archivo.

### Estados y errores que se tratan
Archivo inexistente, vacío, falso, truncado, cifrado, con impresión prohibida o de un
formato no soportado · LibreOffice colgado o sin resultado · CUPS caído · cola
detenida o que no acepta trabajos · impresora desconectada · error de `lp` (traducido) ·
trabajo atascado, cancelado o abortado · terminal demasiado pequeña · rangos inválidos
o fuera del documento · papel desconocido.

---

## 10. Dónde guarda las cosas

| Ruta | Contenido |
|---|---|
| `~/.local/bin/print` | el programa |
| `~/.config/print/config.toml` | configuración (opcional) |
| `~/.cache/print/<clave>/` | PDF convertidos, preparados (`real_*`, `estirar_*`, `pasada*`, `blanco_*`) |
| `~/.cache/print/lo-perfil` y `lo-perfil.lock` | perfil de LibreOffice y su candado |
| `~/.local/state/print/sesion.json` | doble cara pendiente de reanudar |
| `~/.local/state/print/enviados.json` | qué archivo corresponde a cada trabajo enviado (hasta 200) |

La caché se limpia sola: al salir se borra lo de más de 24 h, **salvo** el PDF de un
doble cara pendiente. `print --limpiar` lo borra todo.

Variables de entorno: `PRINT_ANIMACION=0` y `PRINT_ASCII=1` cambian la presentación;
`PRINT_CACHE`, `PRINT_ESTADO` y `PRINT_SYSUSB` redirigen rutas (las usan las pruebas).
`NO_COLOR` y `TERM=dumb` se respetan.

---

## 11. Configuración

`~/.config/print/config.toml` es opcional; hay un ejemplo en
`docs/config.toml.ejemplo`. Prioridad: argumentos > variables de entorno > archivo >
valores por defecto. Un archivo ausente o con valores inválidos nunca bloquea.

| Clave | Por defecto | Valores |
|---|---|---|
| `impresora` | cola predeterminada | nombre de la cola de CUPS |
| `tinta` | `color` | `color` · `normal` · `gris` |
| `animacion` | `true` | |
| `ascii` | `false` | fuerza bordes y símbolos ASCII |
| `giro` | `libro` | `libro` (borde largo) · `tortita` (borde corto) |
| `timeout_soffice` | `60` | segundos |

---

## 12. Pruebas

```bash
go test ./...                              # lógica, CUPS falso y flujos completos
go test -race -count=1 -cover ./...        # con detección de carreras (cobertura ≈ 63 %)
```

Las pruebas de Go usan un CUPS falso (`testdata/bin/`) que simula colas detenidas,
trabajos retenidos, impresora desconectada, `ipptool` caído, LibreOffice colgado, etc.
Hay pruebas de flujo completo de una cara y de doble cara que comprueban **qué páginas
llegan de verdad a cada pasada**. El código se ha sometido a «mutaciones»: se rompe a
propósito cada arreglo importante y se comprueba que alguna prueba falla.

### Pruebas con papel real
Los PDF de `pruebas-papel/` gastan muy poca tinta (contornos finos y números huecos):

| Archivo | Para probar |
|---|---|
| `1-cuatro-paginas-carta.pdf` | una cara, doble cara, reanudar |
| `2-cinco-paginas-impar-carta.pdf` | doble cara con páginas impares |
| `3-a4-una-pagina.pdf` | las cuatro escalas con un A4 |
| `4-apaisado-a4.pdf` | rotación de un PDF horizontal |
| `5-regla-a4-apaisado.pdf`, `6-regla-a4-vertical.pdf` | regla en cm: se ve qué número se corta |

Cada hoja lleva un marco, un cuadrito en cada esquina, la palabra **ARRIBA** con una
flecha y el número de página. Se regeneran con Ghostscript.

---

## 13. Lo que se verificó con papel

Pruebas del 2026-10-17 con la impresora real:

| Prueba | Resultado |
|---|---|
| Una cara, 1 hoja | bien |
| Doble cara, 4 páginas | pila final: 1 arriba, 2 detrás, luego 3 y 4 |
| Doble cara, 5 páginas (impar) | hoja en blanco automática; la 5 queda sola |
| A4 en Carta: Ajustar, Tamaño real, Estirar, Cubrir | cada una hace lo que dice (la regla muestra el recorte) |
| PDF apaisado | acostado hacia el mismo lado en las tres escalas; Tamaño real corta por abajo |
| Reanudar un doble cara | ofrece reanudar y la pasada 2 sale bien |
| Cancelar un trabajo propio | funciona **sin sudo** |

**No se ha probado en papel:** *B/N*, *Sin bordes*, *Tipo papel* distinto de Normal, la
impresión de `.md`/`.csv`/`.html` y retener/liberar un trabajo.

---

## 14. Si algo falla

| Síntoma | Causa probable y solución |
|---|---|
| Aviso «La impresora está deshabilitada» | CUPS detuvo la cola. Pulsa `e` en el menú o `sudo cupsenable Canon_G2110` |
| Aviso «Impresora desconectada o apagada» | Enciéndela o revisa el USB. El aviso se quita solo |
| Imprimir bloqueado | Hay un aviso grave de impresora, o el rango o el doble cara no son válidos: el motivo sale en rojo |
| «La conversión no produjo un PDF» | LibreOffice falló o se colgó. Reintenta con `Enter`; revisa que no haya un `soffice` atascado (`pkill soffice`) |
| «El PDF está dañado o incompleto» | El archivo está cortado. Ábrelo en otro programa y vuelve a guardarlo como PDF |
| «El PDF no permite imprimir» | Tiene una restricción de impresión. Pide una copia sin ella |
| Un `.docx` cambia de número de páginas | Faltan las fuentes de Office: `sudo pacman -S ttf-carlito ttf-caladea` |
| El doble cara sale al revés | En la pantalla de volteo pulsa `g` y repite |
| Tras apagar la impresora, nada sale | La cola se detuvo; habilítala (ver arriba). Los trabajos olvidados salen al habilitarla: cancélalos antes con `x` |
| «No pude preparar las páginas del doble cara» | Falta `qpdf`: `sudo pacman -S qpdf` |
| `print` no se ve bien | Necesita 60×24 como mínimo; `--ascii` si tu terminal no dibuja los símbolos |

Para ver qué haría sin imprimir: `print archivo.pdf --dry-run`. El **mensaje original**
de CUPS se muestra debajo de cada error para poder buscarlo.

---

## 15. Límites conocidos

- **Sin niveles de tinta:** CUPS no los da para esta impresora.
- **Un PDF apaisado en papel vertical** siempre lo gira CUPS (en sentido horario); las
  escalas propias lo respetan.
- **Un `.docx` con contraseña** tarda los 60 s del tiempo máximo antes de fallar.
- **La preparación de «Tamaño real» y «Estirar»** bloquea la pantalla unos segundos con
  PDF muy largos.
- **Doble cara sin terminal** no existe: exige girar las hojas a mano.
- La política de CUPS que detiene la cola si la impresora está apagada **se dejó como
  está** a propósito: así los trabajos olvidados esperan a que decidas, en vez de salir
  solos horas después.
- `.md` y `.txt` se imprimen sin formato (como texto plano).

---

## 16. Decisiones de diseño y por qué

| Decisión | Motivo |
|---|---|
| Go + Bubble Tea | un solo binario y una TUI cómoda en cualquier terminal |
| Solo documentos, nunca imágenes | el objetivo es imprimir documentos, no fotos |
| LibreOffice para convertir | es el motor que creó el archivo, el que menos deforma |
| Perfil de LibreOffice aislado + candado | sin él, una instancia abierta o dos `print` a la vez fallaban |
| Solo opciones verificadas | muchas opciones del controlador son decorativas |
| «Normal» por defecto | `RGB` es el modo estándar; se cambia a `color` en `config.toml` |
| Reversos primero, frentes después | la impresora saca las hojas boca arriba con la última encima |
| Giro por un borde largo | deja las dos caras con la misma orientación |
| Hoja en blanco automática con total impar | evita que el usuario ponga una hoja a mano |
| PDF reordenado con `qpdf` | CUPS solo imprime rangos ascendentes |
| Escalas propias para el PDF apaisado | CUPS siempre lo reduce |
| Comando visible solo con `o` | la interfaz muestra «Papel: Carta», no `PageSize=Letter` |
| Sin casilla ni cuenta atrás en el volteo | «un Enter y listo»; la animación ya explica el giro |
| `ipptool` con `requesting-user-name` | sin él CUPS oculta nombre y dueño |
| Detectar la impresora por el URI de la cola | un ID fijo fallaba con otro puerto o modelo |
| No cambiar la política de errores de CUPS | los trabajos olvidados no deben salir solos |

---

## 17. Pendiente e ideas

**Pendiente:** más pruebas automáticas de la interfaz y de la conversión con LibreOffice.

**Por probar con la impresora:** los puntos de «No se ha probado en papel» de la
sección 13.

**Ideas opcionales:** esconder «Tipo papel» y «Sin bordes» detrás de «Más», un historial
con reimprimir y un atajo de teclado para abrir `print`.

---

## 18. Historial de lo aprendido

Lecciones que costó encontrar y conviene no olvidar:

1. **Comparar solo el raster no basta** para saber si una opción del controlador
   funciona: el filtro final consume opciones que el raster no refleja.
2. **El orden de impresión importa por cómo sale el papel:** boca arriba, última encima.
   El primer diseño dejaba la pila al revés.
3. **CUPS ignora `outputorder=reverse`** con este filtro: hay que reordenar el PDF.
4. **Ghostscript añade `/Rotate` según el texto** y deshace los giros propios. Usar
   `-dAutoRotatePages=/None`.
5. **CUPS reduce siempre un PDF apaisado** en papel vertical, ignorando `print-scaling`.
6. **`ipptool` sin `requesting-user-name`** oculta nombre y dueño de los trabajos.
7. **Los archivos de la caché deben llevar versión en el nombre**, o un cambio de lógica
   sigue sirviendo el resultado viejo.
8. **Las pruebas deben usar datos como los reales** (el PDF de prueba sin texto no
   mostraba el fallo de la rotación) y conviene romper a propósito el código para
   comprobar que una prueba falla.
9. **Los campos de texto de Bubble Tea sin ancho** recortan el texto de ayuda a una
   letra.
10. **Dos procesos no pueden compartir el perfil de LibreOffice** sin un candado.
11. **`cancel` de un trabajo propio funciona sin sudo** en este CUPS.
