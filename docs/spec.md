> **Estado (2026-10-17):** esta es la especificación original. El programa ya está
> hecho y probado con papel; el estado real, el uso y las decisiones vigentes están en
> `README.md`. Las secciones marcadas como «Actualización» (§8) reflejan lo que cambió
> tras probar: giro por borde largo, orden de pasadas, hoja en blanco automática.
> Algunas ideas de aquí **no se hicieron** a propósito (`--calibrar`, el botón
> «Salió mal», la casilla de comprobación y la cuenta atrás del volteo).
>
> **Modelo de referencia:** los valores concretos (nombre de cola, ID USB, URI) son
> el ejemplo con el que se probó. `print` funciona con cualquier cola de CUPS.

# Especificación: CLI-Print — menú interactivo de impresión para Linux

Documento de implementación (v2). Pásaselo a otra IA o úsalo tú mismo como guía.

**Cambios de la v2 respecto a la v1:** se añadió chequeo previo de salud
(§6), un catálogo de fallos con su respuesta (§7), la lógica de doble cara
rehecha de forma general para rangos, copias, impares y horizontal (§8),
persistencia para reanudar (§9), diseño visual y animaciones concretos (§10),
configuración y calibración (§11), y pruebas automáticas (§15).

---

## 1. Contexto del proyecto

El usuario quiere un programa de línea de comandos que funcione como **menú
interactivo de impresión**. Desde él debe poder elegir un documento y decidir
cómo imprimirlo, sin tener que recordar la sintaxis de `lp`.

**Estado actual importante:** la impresora ya está totalmente configurada y
funcionando. Este proyecto **solo añade una interfaz cómoda encima**. No hay que
configurar nada de impresión desde cero.

**Principios de diseño (aplican a todo el documento):**

1. **Nunca fallar en silencio.** Todo error se muestra con causa y con la acción
   concreta que el usuario puede hacer.
2. **Nunca gastar tinta ni papel por una suposición.** Lo no verificado se
   calibra o se pregunta antes.
3. **Degradar con gracia.** Sin color, sin UTF-8, sin animación o sin `pdfinfo`,
   el programa sigue siendo usable.
4. **Sin sudo, sin tocar el sistema.**
5. **Bonito pero sutil.** Animación que informa, no que estorba.

---

## 2. Entorno del sistema (verificado, no suposiciones)

| Elemento | Valor |
|---|---|
| SO | Linux (probado en Arch Linux + Wayland/Hyprland) |
| Usuario | `usuario` (grupos: `usuario`, `scanner`, `wheel`) |
| Go | `1.27.0` instalado en `/usr/bin/go` |
| Acceso a red | `proxy.golang.org` responde HTTP 200 (se pueden descargar módulos) |
| CUPS | `2:2.4.19-1` |
| LibreOffice | `26.8.0.3` (`soffice`, `libreoffice`) |
| Poppler | `pdfinfo` disponible |
| Fuentes instaladas | 918 |
| Idioma del sistema | `LANG=es_PA.UTF-8` (Panamá) |

Ya instalados y **no hay que volver a instalarlos**: `gutenprint 5.3.5-1`,
`foomatic-db-gutenprint-ppds`, `sane`, `simple-scan`, `cups`, `cups-filters`.

Herramientas TUI ya presentes: `fzf`, `whiptail`, `gum` (el proyecto descarta
usarlas y usa Bubble Tea en Go; motivo: control total de animación y estado).

---

## 3. Impresora: datos exactos

- **Modelo:** Canon PIXMA G2110
- **Se identifica por USB como:** `G2010 series` (familia G2000)
- **ID USB:** `04a9:183a`, **número de serie:** `05DBAE`
- **URI del dispositivo:** `usb://Canon/G2010%20series?serial=05DBAE&interface=1`
- **Nombre de la cola CUPS:** `Canon_G2110` (predeterminada y compartida)
- **PPD en uso:**
  `/usr/share/cups/model/gutenprint/5.3/Global/stp-bjc-G2000-series.5.3.sim.ppd.gz`
  (nombre interno: `Canon G2000 series`)
- **Valores por defecto de la cola:** `PageSize=Letter`, `MediaType=Plain`,
  `ColorModel=RGB`, `print-color-mode=color`

### 3.1 Restricciones físicas importantes

- **NO tiene duplexor automático.** El doble cara es siempre manual, en dos
  pasadas. No existe la opción `Duplex` en el PPD.
- **Tinta negra casi agotada.** Las zonas completamente negras salen en blanco
  porque el cartucho no da. Por eso el modo de color por defecto es **solo
  tintas de color (CMY)**, sin negro.
- **Consecuencia de CMY (nueva):** el "negro" se simula mezclando C+M+Y, así que
  el texto negro sale **pardo o gris verdoso**, y consume más tinta de color.
  La interfaz debe avisarlo una vez (ver §7, fallo F-12) y ofrecer la **hoja de
  prueba de negro** (§11.3).
- Los depósitos son transparentes y se pueden ver a simple vista.
- La salida del papel es por delante, **cara impresa hacia arriba** (hipótesis a
  confirmar en calibración, §11.3). Esto determina el orden del doble cara.

### 3.2 Arreglo de conflicto USB ya aplicado

El módulo del kernel `usblp` provocaba que CUPS fallara con
`Failed to re-attach "usblp" kernel module to 04a9:183a` y dejara la
impresora detenida. Ya está resuelto en
`/etc/modprobe.d/blacklist-usbprinter.conf` con `blacklist usblp` y el módulo
fue descargado. **No hay que tocarlo.** El chequeo de salud (§6) sí debe
reconocer ese mensaje si reaparece y explicar la causa.

---

## 4. FASE 0 — Fuentes (hazla primero)

El usuario no tiene instaladas las sustitutas métricamente compatibles de
Microsoft. Sin ellas, los `.docx` que usan Calibri o Cambria **se recolocan**
porque el texto ocupa diferente espacio y el número de páginas puede cambiar.

| Fuente del documento | Qué se usa ahora | ¿Correcto? |
|---|---|---|
| Calibri | Liberation Sans | **No** |
| Cambria | Liberation Serif | **No** |
| Arial | Liberation Sans | Sí |
| Times New Roman | Liberation Serif | Sí |

**Instalar:**
```bash
sudo pacman -S --needed ttf-carlito ttf-caladea
fc-cache -f
fc-match Calibri    # debe responder Carlito
fc-match Cambria    # debe responder Caladea
```

Esto debe hacerse ANTES de construir nada, para probar con documentos reales
sin falsos positivos.

**Fallback dentro de `print`:** al convertir un documento, si `fc-match Calibri`
no devuelve Carlito, mostrar un aviso discreto en la ficha del archivo:
`⚠ Fuentes de Office ausentes: las páginas pueden cambiar. sudo pacman -S ttf-carlito ttf-caladea`.
Es solo un aviso; no bloquea.

---

## 5. FASE 1 — Verificar qué opciones del driver funcionan

**Esta fase es obligatoria y va primera.** Muchas opciones del panel de CUPS son
**decorativas**: se pueden tocar pero no cambian nada de la salida.

### 5.1 Opciones YA VERIFICADAS

**Funcionan:**
- `ColorModel` — `CMY` produce salida distinta (sin negro). `RGB` es el
  comportamiento por defecto (negro + color).
- `PageSize`
- `StpOrientation` — `Portrait`, `Landscape`, `UpsideDown` (Reverse Portrait),
  `Seascape` (Reverse Landscape)
- `page-ranges` (opción de `lp`, no del PPD)
- `copies` (opción de `lp`)

**NO hacen nada (salida byte a byte idéntica):**
- `StpInkType` (`CMYK`, `RGB`, `Gray`) — aunque el rótulo diga "CMY Color", miente
- `StpResolution` (`300dpi` y las demás)
- `StpColorCorrection` (`Desaturated`, `Accurate`, etc.)

**No existe modo economía/borrador.** `StpQuality` solo ofrece `None`
(Manual Control) y `Standard`, **ambos a 600 dpi**.

### 5.2 Opciones que FALTA verificar

- `ColorModel=Gray`, `Black`, `CMYK`, `KCMY` (solo se probó `CMY`)
- `MediaType` y todas sus variantes
- `StpFullBleed` (Borderless) — interesa en tarjetas de visita, no en papel normal
- `StpiShrinkOutput` (Shrink / Crop / Expand) — **afecta directamente** al caso
  "PDF A4 enviado a Letter" (§7, F-09)
- `StpImageType` (None / Text / Graphics / TextGraphics / Photo / LineArt)
- `StpQuality`
- `StpBrightness`, `StpContrast`, `StpSaturation`

### 5.3 Método de verificación

```bash
P=/usr/share/cups/model/gutenprint/5.3/Global/stp-bjc-G2000-series.5.3.sim.ppd.gz

base=$(cupsfilter -m application/vnd.cups-raster -p "$P" prueba.pdf 2>/dev/null | md5sum | cut -c1-12)

for opt in "ColorModel=CMY" "StpFullBleed=True" "StpImageType=Photo"; do
  h=$(cupsfilter -m application/vnd.cups-raster -p "$P" -o "$opt" prueba.pdf 2>/dev/null | md5sum | cut -c1-12)
  if [ "$h" = "$base" ]; then echo "$opt -> IGNORADA"; else echo "$opt -> REAL"; fi
done
```

**Usar un PDF de prueba pequeño** (media página con texto, un degradado y un
bloque negro sólido). A 600 dpi una página genera ~100 MB de raster.

**Limitaciones del método (nuevo):**
- "IGNORADA" por hash idéntico es una prueba fuerte, pero no absoluta: una
  opción podría actuar solo vía comandos de la impresora fuera del raster. Marcar
  como `IGNORADA (probable)` y no ofrecerla.
- "REAL" no significa "útil": comprobar también que el cambio sea el esperado
  (p. ej. `Gray` no debe usar tinta de color). Se mide comparando el volumen de
  datos por plano de tinta, no solo el hash.

### 5.3b Corrección del método (resultado de la Fase 1, 2026-10-16)

**El método de 5.3 tiene un punto ciego.** Comparar la salida de `cupsfilter -m
application/vnd.cups-raster` solo ve lo que cambia el *raster*. Opciones que
consume el filtro final (`rastertogutenprint.5.3`) parecían "idénticas" y no lo
son. Método correcto: generar el raster una vez y pasarlo por el filtro final con
cada opción:

```bash
cupsfilter -m application/vnd.cups-raster -p "$P" prueba.pdf > base.ras
PPD=/tmp/g2000.ppd /usr/lib/cups/filter/rastertogutenprint.5.3 1 u t 1 "StpFullBleed=True" base.ras | md5sum
```

Resultado: `StpFullBleed`, `StpiShrinkOutput`, `StpImageType`, `StpBrightness`,
`StpContrast`, `StpSaturation`, `MediaType` y `ColorModel` **sí cambian la salida**.
`StpQuality`, `StpResolution` y `StpColorCorrection` siguen siendo ignoradas.
**`StpInkType=Gray` también cambia la salida** (contradice §5.1): podría usar solo
tinta negra, así que no se ofrece hasta probarlo en papel. Detalle en
`opciones_verificadas.json`.

### 5.4 Resultado de la Fase 1 como archivo (nuevo)

Guardar el resultado en `~/print/opciones_verificadas.json`:

```json
{
  "ppd": "stp-bjc-G2000-series.5.3.sim.ppd.gz",
  "verificado": "2026-10-16",
  "opciones": {
    "ColorModel=CMY":  { "estado": "real" },
    "ColorModel=Gray": { "estado": "pendiente" },
    "StpFullBleed":    { "estado": "pendiente" }
  }
}
```

`opciones.go` lee ese archivo (embebido con `go:embed`) para decidir qué
botones existen. **Así el menú nunca ofrece algo no verificado**, y re-verificar
tras cambiar de PPD es solo regenerar el JSON.

**Fallback:** si el JSON falta o está corrupto, el menú muestra únicamente las
opciones de la tabla 5.1 (las ya verificadas) y un aviso `ℹ modo conservador`.

### 5.5 Regla de diseño

**El menú contiene solo opciones verificadas como reales.** Si una opción se
revela inútil, no se pone; o se pone marcada como `experimental` con
explicación.

---

## 6. Chequeo previo de salud ("preflight")

Se ejecuta **antes de abrir el menú**, en paralelo, con tiempo máximo de 3 s en
total. Mientras corre, se muestra el spinner de arranque (§10.4). Resultado:
una lista de problemas **bloqueantes** y **avisos**.

| # | Comprobación | Cómo | Si falla |
|---|---|---|---|
| P-1 | `lp`, `lpstat` existen | `exec.LookPath` | **Bloqueante**: "CUPS no está instalado" + `sudo pacman -S cups` |
| P-2 | `cupsd` responde | `lpstat -r` | **Bloqueante**: `sudo systemctl start cups` |
| P-3 | Existe la cola `Canon_G2110` | `lpstat -p Canon_G2110` | Buscar cola predeterminada con `lpstat -d`; si hay otra, usarla y avisar; si no hay ninguna, **bloqueante** |
| P-4 | La cola está habilitada | salida de `lpstat -p` (`enabled`/`disabled`) | **Aviso** con el comando exacto: `sudo cupsenable Canon_G2110`. No se intenta |
| P-5 | La cola acepta trabajos | `lpstat -a` (`accepting`) | **Aviso**: `sudo cupsaccept Canon_G2110` |
| P-6 | Impresora conectada por USB | `lpstat -v` + presencia de `04a9:183a` en `/sys/bus/usb/devices/*/idVendor,idProduct` | **Aviso**: "Conecta o enciende la impresora" |
| P-7 | Error `usblp` | buscar `usblp` en `lpstat -l -p` | **Aviso** con explicación de §3.2 |
| P-8 | `soffice` existe | `LookPath` | Aviso: solo se podrán imprimir PDF |
| P-9 | `pdfinfo` existe | `LookPath` | Fallback a `pdftk`/`qpdf`/`mutool`; si no, leer `/Type /Page` con regex (§7, F-05) |
| P-10 | Fuentes de Office | `fc-match Calibri` | Aviso discreto (§4) |
| P-11 | Terminal interactiva | `term.IsTerminal` | Si no es TTY: modo no interactivo (§12) |
| P-12 | Tinta baja | `lpstat -l -p` / `ipptool` (mejor esfuerzo) | Solo informativo; la G2110 puede no reportarlo |

**Reglas:**
- Los bloqueantes muestran una pantalla de error con la solución y salen con
  código distinto de 0. Nunca un stack trace.
- Los avisos se muestran como una **banda amarilla** en la parte superior del
  menú, y se pueden descartar con `x`. Si hay avisos graves (P-4, P-5, P-6) el
  botón Imprimir se deshabilita con el motivo visible, no se oculta.
- Un comando que se cuelga (p. ej. `lpstat` con CUPS caído) se corta con
  `context.WithTimeout`. Ninguna llamada externa se hace sin timeout.
- `r` (Recargar) vuelve a ejecutar el preflight, para que el usuario arregle algo
  en otra terminal y continúe sin reiniciar `print`.

---

## 7. Catálogo de fallos y respuesta

Cada fila es un requisito de implementación. Código `F-nn` para poder citarlo en
pruebas y mensajes.

### 7.1 Archivos y conversión

| ID | Fallo | Respuesta |
|---|---|---|
| F-01 | La ruta no existe | Mensaje claro; si hay coincidencias parecidas en la carpeta, ofrecer "¿Quisiste decir…?" (distancia de edición) |
| F-02 | La ruta es una imagen o extensión no soportada | "Este programa imprime solo documentos. `.jpg` no está soportado." No se imprime |
| F-03 | Sin permiso de lectura | "No tengo permiso para leer este archivo" + `ls -l` abreviado |
| F-04 | Archivo vacío (0 bytes) | "El archivo está vacío" |
| F-05 | `pdfinfo` falla o no existe | Cadena de fallbacks: `pdfinfo` → `qpdf --show-npages` → `mutool info` → regex sobre `/Type\s*/Page[^s]` → "páginas desconocidas" (se permite imprimir, se desactiva validar rangos) |
| F-06 | PDF protegido con contraseña | Detectar `Encrypted`; avisar y pedir al usuario que lo desbloquee. No se intenta romper nada |
| F-07 | PDF corrupto | Mostrar el error de `pdfinfo` resumido; no continuar |
| F-08 | `soffice` falla, se cuelga o tarda >60 s | Timeout de 60 s con `context`, matar el **grupo de procesos** (`Setpgid` + `syscall.Kill(-pid)`), mostrar la causa y ofrecer reintentar |
| F-08b | `soffice` ya está abierto (el perfil está bloqueado) | Usar siempre un perfil aislado: `-env:UserInstallation=file://<tmp>/lo-profile`. Así nunca choca con una instancia del usuario |
| F-08c | La conversión termina "bien" pero no hay PDF | Verificar que el archivo exista y que `pdfinfo` lo lea; si no, tratar como fallo y mostrar la salida de `soffice` |
| F-08d | Dos conversiones simultáneas | Cola serializada con un único worker; el perfil de LibreOffice no admite concurrencia |
| F-08e | Formato mal soportado: `.tex`, `.epub`, `.md` | `.md`/`.txt`: LibreOffice los abre como texto plano, avisar "se imprimirá sin formato". `.tex`: usar `pdflatex`/`tectonic` si existe, si no, avisar que no se puede. `.epub`: intentar con LibreOffice; si falla, mensaje "formato no soportado de forma fiable". **Hasta probarlos, dejar `.tex` y `.epub` fuera de la lista de extensiones** |
| F-08f | Documento muy largo (>200 páginas) | Aviso de confirmación antes de imprimir: "Son 312 páginas. ¿Seguro?" |
| F-08g | Nombre con caracteres especiales | `exec.Command` con args; para `soffice`, copiar a un nombre temporal seguro (`doc.<ext>`) si el original empieza con `-` o tiene saltos de línea |
| F-08h | Disco/tmp lleno | Detectar el error de escritura y explicarlo |
| F-08i | El archivo cambia mientras está abierto | La caché usa ruta + mtime + tamaño; en cada "Recargar" se revalida |

### 7.2 Papel y opciones

| ID | Fallo | Respuesta |
|---|---|---|
| F-09 | PDF en A4 y la cola en Letter (o al revés) | Detectar el tamaño del PDF. Si el papel elegido ≠ papel del PDF, mostrar **aviso explícito** `PDF A4 → papel Letter: se reducirá o recortará` y ofrecer elegir entre: usar el tamaño del PDF (recomendado), ajustar a la página (`fit-to-page` / `StpiShrinkOutput`, si la Fase 1 lo confirma), o continuar sin escalar |
| F-09b | Tamaño de papel no reconocido por el PPD (p. ej. A3) | Listar solo tamaños que existan en el PPD (`lpoptions -p Canon_G2110 -l`). Si no existe, caer al más cercano y avisar |
| F-09c | PDF con páginas de tamaños mixtos | Usar el de la primera página y avisar "tamaños mezclados" |
| F-10 | Rango inválido (`5-2`, `0`, letras, `1,,3`) | Validación **mientras se escribe**, con el error resaltado en rojo bajo el campo y ejemplo de sintaxis. Imprimir queda deshabilitado |
| F-10b | Rango fuera del total (`1-30` en un doc de 14) | Aviso con opción de recortar automáticamente a `1-14` |
| F-10c | Rango sobre documento de páginas desconocidas | Aceptar la sintaxis, pero avisar que no se pudo validar |
| F-11 | Copias fuera de 1–99 | Limitar con rebote visual; confirmar si es >10 |
| F-12 | CMY con documento de texto negro | Aviso **una sola vez por sesión**: "En modo Solo color el texto negro sale pardo". Con `[t]` ofrece la hoja de prueba (§11.3) y `[n]` cambia a Normal |
| F-12b | Opción PENDIENTE sin verificar | No se muestra (§5.5) |

### 7.3 Impresión y cola

| ID | Fallo | Respuesta |
|---|---|---|
| F-13 | `lp` devuelve código ≠ 0 | Mostrar el `stderr` de `lp` traducido: tabla de errores comunes (`lp: Error - The printer or class does not exist`, `forbidden`, `Unable to connect`) → causa en español + acción. Siempre dejar visible el texto original abajo |
| F-14 | `lp` ok, pero el trabajo queda detenido (`held`, `stopped`) | Seguir el trabajo con su **ID** (`lp` imprime `request id is Canon_G2110-42`), consultar `lpstat -W not-completed -o Canon_G2110` solo para ese ID. Si pasa 20 s sin avanzar, mostrar diagnóstico (P-4/P-5/P-6) |
| F-14b | Atasco de papel / sin papel / tinta (la impresora lo reporta como `printer-state-reasons`) | Leer `printer-state-reasons` por IPP (`ipptool` o `lpstat -l`) y mostrar un mensaje humano: "Sin papel", "Atasco", "Tapa abierta". Reintento con `Enter` |
| F-15 | El trabajo desaparece de la cola pero nada salió | Aclarar al usuario: "CUPS lo marcó completado. Si no salió nada, revisa papel/tinta". No es detectable con certeza |
| F-16 | El usuario quiere cancelar | `cancel <id>` requiere sudo en CUPS 2.4 (§9.3): mostrar el comando exacto para copiar. El programa no lo ejecuta |
| F-17 | Ctrl+C durante conversión | Matar el grupo de procesos, limpiar temporales, salir limpio |
| F-17b | Ctrl+C durante impresión | **No** cancelar trabajos ya enviados: avisar "El trabajo ya está en la cola de CUPS" y salir |
| F-17c | Señales `SIGTERM`/`SIGHUP`/cierre de terminal | Capturarlas con `signal.NotifyContext` y ejecutar la misma limpieza |
| F-18 | Terminal demasiado pequeña (<60×20) | Pantalla `Agranda la terminal (mínimo 60×20)` y se recupera sola al redimensionar (`tea.WindowSizeMsg`) |
| F-19 | Proceso muerto a medias (corte de luz, `kill -9`) | Al próximo arranque, detectar la sesión de doble cara pendiente (§9) y ofrecer reanudarla; borrar temporales viejos (>24 h) del directorio de `print` |

### 7.4 Doble cara

Ver §8; los fallos propios del doble cara (impar, rango, copias, atasco entre
pasadas, orientación horizontal) están resueltos allí.

---

## 8. Lógica del doble cara manual (rehecha, general)

La v1 asumía "impares = anverso, pares = reverso". Eso solo es cierto cuando se
imprime el documento completo y desde la página 1. Esta versión usa una regla
que funciona **con cualquier rango**.

### 8.1 Por qué dos pasadas

La G2110 no tiene duplexor. Se imprime un lado, se voltea el papel a mano y se
imprime el otro lado.

### 8.2 Definición formal

1. **Seleccionar** las páginas pedidas por el usuario y expandirlas en una lista
   ordenada `S`. Ejemplo: rango `2-7` → `S = [2,3,4,5,6,7]`.
2. **Agrupar en hojas** por *posición* en `S`, no por paridad:
   hoja *k* = `(S[2k], S[2k+1])`.
3. **Anversos** `A` = `S[0], S[2], S[4], …` (posiciones pares).
   **Reversos** `R` = `S[1], S[3], S[5], …` (posiciones impares).
4. Pasada 1 imprime `A` en orden ascendente; pasada 2 imprime `R` en orden
   ascendente.

Ejemplos:

| Entrada | `A` (pasada 1) | `R` (pasada 2) | Hojas |
|---|---|---|---|
| Todo, 14 págs | 1,3,5,7,9,11,13 | 2,4,6,8,10,12,14 | 7 |
| Todo, 13 págs (impar) | 1,3,5,7,9,11,13 | 2,4,6,8,10,12 | 7, la última sin reverso |
| Rango `3-8` | 3,5,7 | 4,6,8 | 3 |
| Rango `2-7` | 2,4,6 | 3,5,7 | 3 (¡el anverso es par!) |
| Rango `1-3,7,10-` en 12 págs | 1,3,10,12 | 2,7,11 | `S=[1,2,3,7,10,11,12]` |

### 8.3 Por qué el orden ascendente en el reverso

La impresora entrega **cara impresa hacia arriba**: la primera hoja queda abajo
y la última arriba. Al voltear el bloque "como una tortita" **se invierte el
orden**: la hoja 1 queda arriba y la impresora la toma primero. Por eso el
reverso se pide en orden normal.

En el caso impar, la última hoja (sin reverso) queda **abajo del todo** tras el
volteo, así que la impresora no la toca. Funciona sin trucos; solo hay que
explicárselo al usuario (§8.6).

> **Esta cadena de razonamiento depende de tres hipótesis físicas** (salida
> cara arriba, giro "tortita", la bandeja toma la hoja de arriba). Se confirman
> con la **calibración** de §11.3 antes de la primera impresión real y quedan en
> la configuración como `salida`/`giro`. No se da por hecho.

### 8.4 Orientación del reverso

El reverso va girado 180° respecto al anverso para que el texto quede derecho al
voltear. Tabla:

| Orientación elegida | Pasada 1 (anverso) | Pasada 2 (reverso) |
|---|---|---|
| Vertical | `Portrait` | `UpsideDown` |
| Horizontal | `Landscape` | `Seascape` |

**Pendiente de calibrar:** para páginas horizontales, el giro físico correcto
puede ser por el borde corto (tortita de lado) en lugar de por el largo. La
configuración tiene `giro = "borde_largo" | "borde_corto"`; el texto de la
pantalla de volteo cambia según ese valor. Sin calibrar, el programa muestra el
valor por defecto con la etiqueta `sin calibrar` y ofrece calibrar.

> **Actualización (2026-10-17, tras la primera prueba con papel):** el giro por
> defecto pasó a ser **de libro**: se gira el bloque por uno de sus **bordes
> largos** (los de los lados), a derecha o izquierda, y se vuelve a poner en la
> bandeja igual que al principio. Con ese giro el borde de arriba de la página
> es el mismo en las dos caras, así que en vertical el **reverso va igual que el
> frente** (`Portrait` en las dos pasadas). El giro de calendario (por un borde
> corto) sigue disponible con `giro = "tortita"` en la configuración y entonces
> el reverso va girado 180° (`UpsideDown`). En la pantalla de volteo, `g`
> invierte la orientación del reverso para calibrar con papel real. Con
> contenido apaisado la regla se invierte. **Confirmado con papel (2026-10-17, prueba de 4 páginas).**

> **Actualización 2 (2026-10-17, orden de impresión):** la primera prueba con
> papel mostró que la impresora saca las hojas **boca arriba y la última queda
> encima**. Con "frentes primero, reversos después" la pila final quedaba al
> revés (arriba el 4, al voltearlo el 3). Para que al terminar la **página 1
> quede arriba** (y al voltearla la 2, luego la 3 y su 4) se imprime primero la
> tanda de **reversos** y al final la de **frentes**, cada una **de la última
> hoja a la primera**: con 4 páginas, pasada 1 = `4, 2` y pasada 2 = `3, 1`.
> CUPS solo imprime rangos en orden ascendente (`outputorder=reverse` no lo
> aplica `pdftopdf` aquí), así que `print` reordena el PDF con `qpdf` y envía
> ese archivo entero. Con un total **impar**, la última hoja no tiene reverso
> pero debe estar en el bloque desde la pasada 1: `print` añade una **página en
> blanco al principio de la pasada 1** (la impresora pasa una hoja sin tinta) y
> la pasada 2 empieza por esa página sola; el usuario no pone nada a mano.
> Solo si falta `gs` se le pide añadir la hoja encima del bloque. La orientación girada (si hiciera
> falta) se aplica a la pasada 1, que ahora es la de los reversos.

### 8.5 Combinaciones (todas deben quedar cubiertas)

**Copias con doble cara.** `lp -n 3` mezclaría las copias entre sí y rompe el
orden de las hojas. Estrategia:
- Por defecto, **un ciclo completo por copia**: copia 1 (pasada 1 → volteo →
  pasada 2), copia 2, etc. Se muestra "Copia 2 de 3".
- Para documentos cortos hay un atajo: imprimir `A` repetido *n* veces con
  `copies=n` y `Collate=true` **solo si** la verificación de §5 confirma que
  intercala bien; si no, se usa el ciclo por copia.
- Si `copies > 3`, avisar que el proceso será largo y pedir confirmación.

**Rangos.** Se aplican antes de dividir en `A`/`R` (ver §8.2). Los rangos de
`lp` (`page-ranges`) se generan **ya compactados** (`1,3,5-9` se genera como
`1,3,5,6,7,8,9` → compactado a `1,3,5-9`), y se verifica que `lp` respete el
orden: **`page-ranges` imprime en orden del documento**, nunca reordena. Por eso
solo se pueden pedir listas ascendentes, que es lo que usamos.

**Número impar de hojas / páginas.** Se avisa **antes de empezar**, en la vista
previa y en el volteo: `La última hoja (pág. 13) queda sin reverso; déjala en el
bloque, la impresora no la usará.`

**Una sola hoja (≤2 páginas).** El doble cara se ofrece igual, pero se avisa que
no ahorra pasadas relevantes. Con 1 página, "Doble" se desactiva.

**Documento con página sola en el rango.** Si `S` tiene longitud 1 → `R` vacío:
se omite la pantalla de volteo y se imprime en una cara.

### 8.6 Pantalla de volteo: estados y fallbacks

Máquina de estados (cada estado es testeable sin impresora):

```
PASADA1_ENVIADA -> PASADA1_IMPRIMIENDO -> PASADA1_TERMINADA
   -> ESPERA_VOLTEO (pausa, sin cuenta atrás)
   -> USUARIO_LISTO (Enter) -> CONFIRMA_BLOQUE
   -> PASADA2_ENVIADA -> PASADA2_IMPRIMIENDO -> TERMINADO
```

Reglas:
- **No avanza solo.** Se pasa de `ESPERA_VOLTEO` a `PASADA2_ENVIADA` solo con
  una confirmación explícita del usuario. Si el usuario se distrae, la
  impresora no recibe nada.
- La "cuenta atrás" de la v1 se reemplaza por un **temporizador de cortesía**:
  tras pulsar `Enter`, hay 5 s visibles en los que `Esc` cancela antes de que se
  envíe la pasada 2. Esto evita envíos por accidente sin obligar a esperar.
- **Comprobación visual obligatoria:** la pantalla muestra una mini ilustración
  y pide marcar `[x] La hoja de la pág. 1 está ARRIBA del todo` antes de
  habilitar el envío. En la segunda impresión del mismo día el usuario puede
  desactivarlo en la configuración.
- **Espera de la pasada 1:** no se muestra la pantalla de volteo hasta que
  `lpstat` indique que el trabajo ya **terminó de imprimir** (no solo que se
  encoló). Si no se puede saber (F-15), se muestra un botón `Ya terminó de
  imprimir` que el usuario pulsa.
- **Si la pasada 1 falla a medias** (atasco, sin papel): se aborta el flujo,
  se explica qué páginas **sí** salieron (por la cola) y se ofrece reimprimir
  desde una página (`S` recortada).
- **Si el usuario sale entre pasadas** (Ctrl+C, cierra la terminal): se guarda
  el estado (§9). La próxima vez, `print` ofrece "Reanudar pasada 2 de
  `informe.pdf`".
- **Mal volteo:** si el usuario nota que salió espejado o invertido, hay un
  botón `Salió mal` que sugiere el giro opuesto, y guarda el resultado en la
  calibración. Se reimprime solo la pasada 2.

---

## 9. Persistencia, sesiones y limpieza

### 9.1 Estado de la sesión

Archivo `~/.local/state/print/sesion.json`, escrito de forma **atómica** (escribir
a `.tmp` y `os.Rename`) tras cada cambio de estado importante:

```json
{
  "id": "2026-10-16T14:22:08",
  "archivo": "~/Descargas/informe.pdf",
  "mtime": 1760624528,
  "pdf": "~/.cache/print/ab12cd/informe.pdf",
  "paginas": 13,
  "A": [1,3,5,7,9,11,13],
  "R": [2,4,6,8,10,12],
  "estado": "ESPERA_VOLTEO",
  "job_pasada1": "Canon_G2110-42",
  "copia": 1,
  "copias": 1,
  "opciones": { "PageSize": "A4", "ColorModel": "CMY", "StpOrientation": "Portrait" }
}
```

Al arrancar, si hay sesión no terminada y el archivo sigue existiendo con el
mismo `mtime`, se ofrece reanudar. Si el archivo cambió, se descarta con aviso.
Una sesión con más de 24 h se descarta sin preguntar.

### 9.2 Caché de conversión

- Clave: `sha256(ruta absoluta + mtime + tamaño)`.
- Directorio: `~/.cache/print/<clave>/` (no en `/tmp`, para que reanudar una
  sesión sobreviva a un reinicio).
- Limpieza: al salir se borra lo de la ejecución actual **salvo** el PDF de una
  sesión de doble cara pendiente; al arrancar se borra lo anterior a 24 h.
- Si la limpieza falla, se ignora (nunca bloquear la salida por eso).

### 9.3 Permisos

- **NO** hace falta sudo para imprimir (`lp` funciona como usuario normal).
- **SÍ** hace falta sudo para habilitar la impresora (`cupsenable`) y cancelar
  (`cancel`), porque CUPS 2.4 lo prohíbe por defecto. **El programa no debe
  necesitar sudo.** Si algo requiere sudo, se muestra el comando exacto en un
  recuadro con la tecla `c` para **copiarlo al portapapeles** (`wl-copy`, con
  fallback a `xclip`, y si no hay ninguno, solo se muestra).
- El usuario ya está en el grupo `scanner`.

---

## 10. Diseño visual y animaciones

### 10.1 Paleta y estilo

Basado en `lipgloss`. Detectar el soporte de color con `termenv`:

| Capacidad | Resultado |
|---|---|
| TrueColor | Degradados reales y paleta completa |
| 256 colores | Paleta aproximada, sin degradado |
| 16 colores | Solo acentos básicos |
| `NO_COLOR` o sin TTY | Monocromo, con símbolos para distinguir estado |

Paleta (TrueColor), pensada para fondos oscuros y que
**respeta el tema**: preferir colores ANSI del terminal para el texto y usar
TrueColor solo para acentos.

| Rol | Uso | Color sugerido |
|---|---|---|
| Acento | marco, selección | cian `#7DCFFF` |
| Acento 2 | progreso, título | violeta `#BB9AF7` |
| Éxito | check, listo | verde `#9ECE6A` |
| Aviso | banda amarilla | ámbar `#E0AF68` |
| Error | fallos | rojo `#F7768E` |
| Tinta (C/M/Y) | indicador de tinta | `#00BFFF` / `#FF4FA3` / `#FFD93D` |
| Texto tenue | ayudas, atajos | gris `#565F89` |

Reglas de estilo:
- Bordes redondeados (`lipgloss.RoundedBorder()`), relleno interno 1×2.
- El título `print` lleva degradado horizontal cian→violeta.
- Un **icono de impresora** en ASCII/Unicode en el encabezado.
- Cada opción tiene un **icono** (con fallback ASCII si no hay Nerd Font ni
  UTF-8, §10.6).
- Ancho adaptable: el contenido se centra y limita a 76 columnas.

### 10.2 Maqueta del menú principal

```
╭─ print ─────────────────────────────────────────────────────────╮
│  ⎙  Canon PIXMA G2110            ● Lista     C ▮▮▮▮▮  M ▮▮▮▮▯  Y ▮▮▮▯▯  │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│   ▤ Guia_Estudio_Software_I.docx                             │
│      DOCX · 14 páginas · 1.2 MB · A4                          │
│                                                               │
│   ┌ Opciones ───────────────────────────────────────────────┐ │
│   │ ▸ Tinta        ◉ Solo color   ○ Normal   ○ B/N          │ │
│   │   Papel        A4  (detectado del PDF)                  │ │
│   │   Caras        ○ Una   ◉ Doble (manual)                 │ │
│   │   Páginas      ◉ Todas  ○ Rango  ____                   │ │
│   │   Copias       ‹ 1 ›                                    │ │
│   │   Orientación  ◉ Vertical   ○ Horizontal                │ │
│   └─────────────────────────────────────────────────────────┘ │
│                                                               │
│   ⓘ Doble cara: 7 hojas · 2 pasadas · pasada 1 = págs 1,3,5…  │
│                                                               │
├───────────────────────────────────────────────────────────────┤
│  ↑↓ mover  ←→ cambiar  espacio imprimir  e editar  r recargar  q salir │
╰───────────────────────────────────────────────────────────────╯
```

(La barra de tinta es **solo informativa y mejor esfuerzo**, §6 P-12. Si no se
puede leer, se oculta; nunca se inventa un valor.)

La maqueta usa símbolos ASCII/box-drawing en este documento; **la interfaz real
lleva tildes y ortografía completa** (`Páginas`, `Orientación`, etc.).

### 10.3 Resumen vivo debajo de las opciones

Una línea que se actualiza al cambiar cualquier opción y explica el resultado
en lenguaje natural:

`→ 7 hojas, 2 pasadas, en A4 vertical, solo tintas de color. Estimado: ~3 min.`

Esto evita que el usuario deba descifrar el comando.

### 10.4 Catálogo de animaciones

Todo con `bubbles` y `harmonica` (resortes). Refresco a 60 ms por trama, sin
bucles de CPU: solo `tea.Tick`.

| Momento | Animación | Duración |
|---|---|---|
| Arranque | Spinner tipo `Dot` + texto "Revisando impresora…" que va marcando ✓ en cada chequeo del preflight | ≤3 s |
| Abrir el menú | Marco que se **dibuja progresivamente** (esquinas → bordes), luego el contenido aparece fila por fila (fade-in por línea) | ~250 ms |
| Cambiar de pantalla | **Deslizamiento horizontal** de 3 tramas, usando `harmonica` para suavizar | ~150 ms |
| Mover la selección | El cursor `▸` se desplaza con resorte, la fila activa se resalta | ~100 ms |
| Cambiar una opción | Pulso breve (brillo) en el valor elegido | ~120 ms |
| Convertir a PDF | Spinner + barra **indeterminada** + texto rotativo ("Abriendo documento…", "Maquetando…", "Generando PDF…") | hasta 60 s |
| Buscar archivos | Lista que se filtra con **resaltado de las letras coincidentes** | instantáneo |
| Enviar a imprimir | Barra de progreso real basada en el estado del trabajo en CUPS (si se conoce), si no, indeterminada | variable |
| Papel saliendo | Animación ASCII de la hoja avanzando al imprimir | continua |
| Volteo | Flechas giratorias (ver §10.5) | continua |
| Éxito | `✓` que se **dibuja** y parpadea en verde una vez | ~400 ms |
| Error | Pequeña **sacudida** horizontal (2 ciclos) del recuadro de error | ~200 ms |
| Aviso | La banda amarilla entra deslizándose desde arriba | ~150 ms |

**Reglas de las animaciones:**
- **Informan, no decoran:** cada una acompaña un cambio de estado real.
- Siempre cancelables: cualquier tecla durante una transición la salta.
- **Modo sin animación** (§10.6): `PRINT_ANIMACION=0`, `--sin-animacion`,
  `NO_COLOR`, `TERM=dumb`, conexión SSH lenta o no-TTY. El estado se muestra con
  texto fijo.
- Ninguna animación bloquea la entrada del usuario ni espera a terminar.

### 10.5 Pantalla de volteo (maqueta)

```
╭─ Voltea las hojas ───────────────────────────────────────────╮
│                                                              │
│        pasada 1 de 2 terminada  ✓                            │
│                                                              │
│              ┌────────┐        ┌────────┐                    │
│              │▒▒▒▒▒▒▒▒│  ⟳     │        │                    │
│              │▒▒ 1 ▒▒▒│ ───▶   │   ·    │      ← giro        │
│              │▒▒▒▒▒▒▒▒│        │        │        "tortita"   │
│              └────────┘        └────────┘                    │
│                                                              │
│   1. Saca las hojas de la bandeja trasera                    │
│   2. Gira el bloque entero 180° de delante hacia atrás       │
│   3. Comprueba: la hoja de la página 1 debe quedar ARRIBA    │
│   4. Vuelve a ponerlas en la bandeja                         │
│                                                              │
│   [x] Comprobado: la página 1 está arriba                    │
│                                                              │
│   ⓘ La última hoja (pág. 13) queda sin reverso.              │
│                                                              │
│       Enter  imprimir pasada 2        Esc  esperar           │
╰──────────────────────────────────────────────────────────────╯
```

Animación: el rectángulo de la hoja gira en 4–6 tramas (`┌─┐`, `╒═╕`, etc.) con
una flecha curva `⟳` que cambia de ángulo. Al marcar la casilla, el botón
`Enter` pasa de gris a destacado con un pulso.

### 10.6 Degradación visual

| Situación | Comportamiento |
|---|---|
| `LANG` sin UTF-8 | Bordes ASCII (`+ - |`), spinner `\|/-`, sin emojis |
| Sin Nerd Font | Iconos Unicode estándar (`▸ ✓ ✗ ⚠`); si el terminal no los dibuja bien, ASCII (`> OK X !`) |
| Ancho < 76 | Layout compacto de una columna; si < 60, pantalla F-18 |
| Alto < 24 | Sin ilustraciones, solo texto |
| Símbolo no dibujado por la fuente | Se sustituye por ASCII (`⎙`→`[P]`, `▤`→`[D]`) |

**Decisión:** no se usan emojis en ninguna parte; solo símbolos Unicode simples
(`▸ ◉ ○ ✓ ✗ ⚠ ⓘ ⎙ ▤ ⟳`).

Detección: variable de entorno `PRINT_ASCII=1` para forzarlo y `--ascii` en línea
de comandos.

### 10.7 Accesibilidad

- Todo estado tiene **texto además de color** (nunca solo color).
- Atajos visibles en el pie de pantalla, siempre.
- `?` abre una ayuda contextual a pantalla completa.
- Compatible con lectores de pantalla en modo `--sin-animacion` (sin redibujar
  la pantalla completa en cada trama).

---

## 11. Configuración y calibración

### 11.1 Archivo de configuración

`~/.config/print/config.toml`, **opcional**. Si no existe o es inválido se usan los
valores por defecto y se avisa (sin bloquear).

```toml
impresora      = "Canon_G2110"
papel          = "auto"        # auto = tamaño detectado del PDF
tinta          = "color"       # color | normal | gris
animacion      = true
ascii          = false
confirmar_voltea = true        # exigir la casilla de comprobación
salida         = "cara_arriba" # cara_arriba | cara_abajo  (calibrable)
giro           = "borde_largo" # borde_largo | borde_corto (calibrable)
timeout_soffice = 60
```

Prioridad: **argumentos > variables de entorno > archivo > valores por defecto**.

### 11.2 Argumentos de línea de comandos

```
print [opciones] [ruta]

  --dry-run          muestra la orden `lp` y no imprime nada
  --sin-animacion    desactiva animaciones
  --ascii            fuerza modo ASCII
  --debug            registra todo en ~/.local/state/print/debug.log
  --calibrar         abre el asistente de calibración (§11.3)
  --reanudar         reanuda la sesión pendiente
  --version, --help
```

### 11.3 Asistente de calibración (una vez, ahorra tinta después)

Verifica las hipótesis físicas del §8 con **una sola hoja** y poca tinta:

1. **Prueba de volteo:** imprime una hoja con una pasada 1 que dice "FRENTE ·
   PÁGINA 1 · ARRIBA ↑" y, tras el volteo guiado, una pasada 2 que dice "REVERSO
   · PÁGINA 2". El usuario responde dos preguntas: ¿el reverso salió derecho?
   ¿coincide con el frente al contraluz? Con la respuesta se fijan `salida` y
   `giro`.
2. **Prueba de orden:** con 2 hojas (4 páginas) confirma que el orden del
   reverso es ascendente. Consume 2 hojas.
3. **Prueba de negro (opcional):** imprime un bloque de texto en `CMY` y otro en
   `RGB` para que el usuario decida si el pardo es aceptable.

Todo texto de prueba se genera dentro del programa como PDF mínimo, sin
dependencias. Los resultados se guardan en `config.toml`.

---

## 12. FASE 2 — Especificación del programa

### 12.1 Requisitos técnicos

- **Lenguaje:** Go
- **TUI:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) +
  [Lipgloss](https://github.com/charmbracelet/lipgloss) +
  [Bubbles](https://github.com/charmbracelet/bubbles) (spinner, progress,
  textinput) + [Harmonica](https://github.com/charmbracelet/harmonica)
  (animación con resortes) + [termenv](https://github.com/muesli/termenv)
  (detección de color). Todas MIT.
- **Binario:** un solo archivo, en `~/.local/bin/print`
- **Código:** en `~/print/`
- **Sin cgo:** compilar con `CGO_ENABLED=0` para un binario estático.

### 12.2 Uso esperado

```bash
print                                  # abre el menú; se busca el archivo dentro
print ~/Descargas/Guia.docx            # va directo a la info de ese archivo
print ~/Descargas/                     # navegas el contenido de esa carpeta
print "informe final (v2).docx"        # ruta relativa con espacios
print --dry-run informe.pdf            # solo muestra el comando
```

Debe aceptar rutas relativas, `~`, enlaces simbólicos y espacios.

### 12.3 Modo no interactivo (nuevo)

Si no hay TTY (tubería, script, cron): no abrir la TUI. Imprimir con las
opciones por defecto o las dadas por argumentos, o salir con un error claro. Así
`print` se puede usar en scripts sin colgarse esperando teclas.

### 12.4 Alcance: SOLO DOCUMENTOS

Se pidió explícitamente **solo documentos, nada de imágenes**. Si el usuario
pasa una ruta a un `.jpg`, avisa con un mensaje claro (F-02) y no lo imprime.

Extensiones soportadas de entrada:

```
pdf  docx  doc  odt  ods  odp  pptx  ppt  xlsx  xls
rtf  txt  md  markdown  html  htm  csv
```

`epub` y `tex` quedan **fuera hasta probarlos** (F-08e). La detección usa la
extensión **y** el tipo MIME real (`file --mime-type` o sniffing de los primeros
bytes) para evitar que un `.pdf` renombrado engañe.

**Regla:** el buscador muestra **solo** archivos de esas extensiones. Las
imágenes no se muestran en absoluto.

### 12.5 Conversión a PDF

Todo termina siendo un PDF:

- **Si el archivo es `.pdf`:** se lee directo con `pdfinfo` (rápido).
- **Si es cualquier otro formato:** se convierte con LibreOffice headless, con
  perfil aislado y timeout:
  ```bash
  soffice -env:UserInstallation=file:///tmp/print-lo-profile \
          --headless --norestore --convert-to pdf --outdir <dir-temp> <archivo>
  ```
  Tarda 2–5 s → **spinner animado** con mensajes rotativos.

**Por qué LibreOffice y no otra cosa** (decisión ya justificada): las
alternativas (`pandoc`, `abiword`, `wkhtmltopdf`) no están instaladas y además
son **peores**, porque reconstruyen el documento con otro motor y cambian más
la maquetación.

La conversión no añade pérdida de fidelidad: al imprimir un `.docx` desde
LibreOffice, este *ya* genera un PDF/PostScript antes de mandarlo a la
impresora. El PDF que genera `print` es el mismo.

Caché y limpieza: ver §9.2. Fallos: ver F-08.

### 12.6 Flujo de pantallas

```
[0] ARRANQUE            preflight con spinner (§6)
[0b] REANUDAR?          si hay sesión pendiente (§9.1)
[1] BUSCADOR            sin argumentos
    |- búsqueda difusa mientras escribes, con letras resaltadas
    |- navegación con flechas, vista previa del nombre/tamaño/fecha
    `- Enter para elegir
[2] ANALIZANDO           spinner
    |- PDF -> leer páginas y papel (instantáneo)
    `- otro -> "Convirtiendo a PDF…" -> leer páginas y papel
[3] MENÚ DE OPCIONES     pantalla principal (§10.2)
[4] VISTA PREVIA         la orden exacta que se va a ejecutar
[5] VOLTEO (solo doble cara)   §10.5 y §8.6
[6] IMPRIMIENDO          barra de progreso por trabajo
[7] LISTO                check animado + resumen + ID del trabajo
```

Desde [3], `Esc` vuelve a [1]. Desde cualquier pantalla `Ctrl+C` hace una salida
limpia (F-17).

### 12.7 Información del archivo a mostrar

- nombre completo
- extensión (DOCX, PDF, ODT…)
- número de páginas (solo si se pudo determinar)
- tamaño en disco
- tamaño de papel detectado
- aviso de fuentes ausentes (si aplica)

### 12.8 Opciones del menú y su estado

| Opción | Traducción a CUPS | Estado |
|---|---|---|
| Impresora | `lpstat -p` | solo `Canon_G2110` por ahora |
| Papel | `PageSize` | **REAL** |
| Tinta: Solo color | `ColorModel=CMY` | **REAL**, predeterminado |
| Tinta: Normal | `ColorModel=RGB` | **REAL** |
| Tinta: B/N | `ColorModel=Gray` | **PENDIENTE (Fase 1)** |
| Tipo papel | `MediaType` | **PENDIENTE (Fase 1)** |
| Caras | doble cara manual | ver §8 |
| Páginas | `page-ranges` | **REAL** |
| Copias | `-n` | **REAL** |
| Orientación | `StpOrientation` | **REAL** |
| Sin bordes | `StpFullBleed` | **PENDIENTE (Fase 1)** |
| Ajustar a página | `StpiShrinkOutput` / `fit-to-page` | **PENDIENTE (Fase 1)** |

Las marcadas como PENDIENTE solo se muestran si la Fase 1 confirma que
funcionan (§5.4). "Calidad" (600 dpi fijo) y "Resolución" **no se ofrecen**.

### 12.9 Rangos de página

Aceptar sintaxis humana y traducirla:

- `1-3` → `page-ranges=1-3`
- `1,5,9` → `page-ranges=1,5,9`
- `1-3,7,10-` → `page-ranges=1-3,7,10-`
- Espacios y mayúsculas se toleran (`1 - 3 , 7`).
- Se **normaliza**: se ordena, se fusionan solapes y se eliminan duplicados
  (`3-5,4-8,1` → `1,3-8`).

Validación en vivo (F-10, F-10b, F-10c). Todo en una función pura
`ParseRangos(texto, total) -> ([]int, error)` con pruebas unitarias.

### 12.10 Vista previa antes de imprimir

Antes de mandar nada a la impresora, mostrar la orden exacta:

```
Imprimiendo (pasada 1 de 2):
  lp -d Canon_G2110 -o PageSize=A4 -o ColorModel=CMY \
     -o StpOrientation=Portrait -o page-ranges=1,3,5,7,9,11,13 \
     ~/Documentos/informe.pdf
```

**Edición (definida):** `e` permite editar **solo las opciones `-o`**, como una
lista `clave=valor` en un campo de texto. El programa las parsea, valida que la
clave exista en el PPD (`lpoptions -l`) y las pasa como argumentos separados. La
ruta del archivo y el nombre de la cola **no son editables** (evita inyección y
errores). Para depurar, `y` copia la orden al portapapeles.

---

## 13. Reglas de implementación

### 13.1 Seguridad al invocar `lp`

**Nunca construir la orden como texto y pasarla a una shell.** Usar `os/exec`
con argumentos separados:

```go
// MAL - inyección de shell si el nombre tiene ; o $ o `
cmd := exec.Command("sh", "-c", "lp "+ruta)

// BIEN
args := []string{"-d", "Canon_G2110", "-o", "PageSize=A4", "--", ruta}
cmd := exec.CommandContext(ctx, "lp", args...)
```

- Poner `--` antes de la ruta para que un archivo llamado `-algo.pdf` no se
  interprete como opción.
- Validar las claves `-o` contra una lista blanca (las del PPD) y los valores
  contra los que el PPD admite.
- Los nombres con espacios, tildes y signos funcionan con args separados.

### 13.2 Invocar `lp` y seguir el trabajo

`lp` es asíncrono: encola y devuelve. Capturar `stdout` (no volcarlo a la
terminal, rompería la TUI) y extraer el ID:

```go
out, err := cmd.CombinedOutput()
// "request id is Canon_G2110-42 (1 file(s))"
id := regexp.MustCompile(`request id is (\S+)`).FindStringSubmatch(string(out))
```

Luego consultar cada 1 s **solo ese trabajo**:

```bash
lpstat -W not-completed -o Canon_G2110     # filtrar por el ID
lpstat -W completed   -o Canon_G2110       # para detectar que ya terminó
```

Estados a traducir: `pending` → "En cola", `processing` → "Imprimiendo",
`held` / `stopped` → diagnóstico (F-14), desaparece de `not-completed` y
aparece en `completed` → "Listo".

### 13.3 Concurrencia (Bubble Tea)

- Todo trabajo lento (soffice, pdfinfo, lpstat) corre en un `tea.Cmd`, nunca en
  `Update`, para que la interfaz no se congele.
- Cada `tea.Cmd` lleva su `context` para poder cancelarlo.
- Los mensajes de resultado incluyen un **ID de petición** para descartar
  respuestas obsoletas (p. ej. si el usuario cambia de archivo mientras se
  convierte el anterior).

### 13.4 Registro (log)

Con `--debug`, escribir en `~/.local/state/print/debug.log`: comandos ejecutados,
códigos de salida, `stderr` y tiempos. Rotación simple (máx. 1 MB). Nunca se
registra el contenido de los documentos.

---

## 14. Estructura sugerida del código

```
~/print/
├─ go.mod
├─ main.go              entrada, argumentos, señales
├─ modelo.go            tipos: Documento, Opciones, Impresora, Sesion
├─ config.go            lectura/escritura de config.toml
├─ preflight.go         chequeos P-1..P-12 (§6)
├─ errores.go           tabla de errores -> mensaje + acción (§7)
├─ descubrir.go         listado de documentos y búsqueda difusa
├─ analizar.go          pdfinfo y cadena de fallbacks (F-05)
├─ convertir.go         LibreOffice headless, perfil aislado, caché
├─ opciones.go          catálogo de opciones (lee opciones_verificadas.json)
├─ rangos.go            ParseRangos y normalización (función pura)
├─ duplex.go            cálculo de A/R, orientación, copias (función pura)
├─ construir.go         construcción del comando lp final
├─ cups.go              lp, lpstat, seguimiento de trabajos
├─ sesion.go            persistencia y reanudación (§9)
├─ calibrar.go          asistente de calibración (§11.3)
├─ tema.go              paleta, estilos, detección de color (§10.1)
├─ anim.go              resortes, transiciones, sacudida, pulso (§10.4)
├─ ui_arranque.go       preflight con spinner
├─ ui_lista.go          buscador
├─ ui_menu.go           pantalla principal de opciones
├─ ui_vista.go          vista previa del comando
├─ ui_volteo.go         pantalla de volteo
├─ ui_imprimiendo.go    progreso del trabajo
├─ ui_listo.go          confirmación
├─ ui_error.go          pantalla de error con acción sugerida
├─ opciones_verificadas.json
└─ *_test.go
```

Las funciones de lógica (`rangos`, `duplex`, `construir`, `errores`) son **puras
y sin I/O**, para probarlas sin impresora ni terminal.

---

## 15. Pruebas

### 15.1 Automáticas (sin gastar tinta)

- `rangos_test.go`: tabla de entradas válidas e inválidas, normalización,
  límites, rangos abiertos (`10-`).
- `duplex_test.go`: la tabla de §8.2 completa (todo, impar, rango par, rango
  mixto, 1 página, 2 páginas, horizontal, copias).
- `construir_test.go`: **pruebas doradas** (golden) del comando `lp` generado
  para cada combinación de opciones; verifica `--`, escape de nombres y orden
  estable de argumentos.
- `errores_test.go`: cada salida típica de `lp`/`lpstat` se traduce al mensaje
  correcto.
- **Impresora falsa:** un script `fake-lp` en el `PATH` que registra los
  argumentos y simula estados (`held`, `stopped`, error). Permite probar F-13,
  F-14 y toda la máquina de estados del doble cara.
- `soffice` falso que se cuelga: prueba F-08 (timeout y limpieza del grupo de
  procesos).
- Pruebas de **modelo de Bubble Tea** con `teatest`: enviar mensajes y comprobar
  la vista, sin terminal real.
- Prueba de **terminal pequeña** y de **sin color** comparando contra salidas
  esperadas.

### 15.2 Manuales (sí gastan tinta)

Archivos reales del usuario:

- `~/Descargas/guia_estudio_charla7_carta_color.pdf` - PDF, 13 págs, Carta
- `~/Descargas/Guia_Estudio_Software_I.docx`
- `~/Descargas/Informe_Laboratorio_4.odt`
- `~/Descargas/parsing en lenguaje natural.docx`

Comprobar:
1. Búsqueda y filtrado de documentos
2. Análisis de PDF (páginas y papel correctos)
3. Conversión de `.docx` y `.odt` a PDF, con número de páginas correcto
4. Vista previa antes de imprimir
5. Impresión de una sola cara en CMY
6. **Calibración** (§11.3) antes de cualquier doble cara real
7. Doble cara manual completo, **verificando el orden real de las hojas**
8. Nombres con espacios y tildes
9. Reanudar tras cerrar entre pasadas
10. Impresora apagada y papel agotado (mensajes de F-14b)

**Aviso:** la impresión gasta tinta de color, que es lo que le queda al usuario.
Hacer todas las pruebas de doble cara con un **PDF de 4 páginas**. El orden del
reverso y el giro solo se confirman con papel de verdad, y por eso existe la
calibración.

---

## 16. Instalación y desinstalación

**Instalar:**
```bash
cd ~/print
CGO_ENABLED=0 go build -ldflags="-s -w" -o ~/.local/bin/print .
```

**Desinstalar:**
```bash
rm ~/.local/bin/print
rm -rf ~/print
rm -rf ~/.config/print ~/.local/state/print ~/.cache/print
```

Fuera de esas rutas no se toca el sistema. `~/.local/bin` ya está en el `PATH`
(comprobado). Los directorios de configuración, estado y caché solo se crean al
usarlos, y `print --limpiar` los borra.

---

## 17. Limpieza pendiente

Borrar el script antiguo **solo después** de que la prueba real de doble cara
(§15.2, punto 7) pase correctamente:

```bash
rm ~/.local/bin/doble-cara
```

`doble-cara` está en `~/.local/bin/`. Se borra porque `print` lo absorbe entero.

---

## 18. Resumen de decisiones ya tomadas

| Decisión | Motivo |
|---|---|
| Go + Bubble Tea | pedido explícitamente por el usuario |
| Solo documentos | pedido explícitamente |
| LibreOffice para convertir | única opción instalada, y la que menos deforma |
| Perfil aislado de LibreOffice + timeout | evita bloqueos con instancias abiertas |
| Solo opciones verificadas (JSON) | muchas opciones del driver son falsas |
| CMY por defecto, con aviso del pardo | la tinta negra del usuario está agotada |
| Sin sudo, comando copiable | CUPS lo prohíbe; el programa avisa, no intenta |
| `exec.Command` con args y `--` | evita inyección de shell y de opciones |
| Detección automática de papel + aviso A4/Letter | la cola usa Carta y los PDF varían |
| Doble cara por posición en `S`, no por paridad | funciona con cualquier rango |
| Volteo sin avance automático | no cargarle hojas al usuario si se distrae |
| Calibración antes del primer doble cara | las hipótesis físicas no se dan por ciertas |
| Sesión persistente y reanudable | un corte entre pasadas no pierde el trabajo |
| Animación sutil, desactivable | informa un cambio de estado, no estorba |

---

## 19. Lo primero que hay que hacer

1. **Fase 0:** instalar `ttf-carlito` y `ttf-caladea`; comprobar con `fc-match`.
2. **Fase 1:** verificar con el método de §5.3 todas las opciones pendientes y
   generar `opciones_verificadas.json`. **Ese resultado decide qué botones van
   en el menú.**
3. **Lógica pura primero:** `rangos.go`, `duplex.go`, `construir.go` con sus
   pruebas. No necesitan impresora ni interfaz.
4. **Capa CUPS y preflight** con la impresora falsa.
5. **Interfaz** pantalla por pantalla, empezando por el menú; las animaciones
   al final, con el modo sin animación funcionando desde el principio.
6. **Calibración** y solo después las pruebas con papel real.
