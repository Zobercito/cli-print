package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Impresora      string
	Tinta          string
	Calidad        string
	Animacion      bool
	ASCII          bool
	Giro           string
	TimeoutSoffice int
}

func configPorDefecto() Config {
	return Config{"", "normal", "normal", true, false, "libro", 60}
}

// CargarConfig lee ~/.config/print/config.toml (clave = valor). Si falta o es
// inválido se usan los valores por defecto: la cola predeterminada del sistema
// y el modo de color configurado.
func CargarConfig() Config {
	c := configPorDefecto()
	if h, err := os.UserHomeDir(); err == nil {
		if datos, err := os.ReadFile(filepath.Join(h, ".config", "print", "config.toml")); err == nil {
			leerConfig(datos, &c)
		}
	}
	if strings.TrimSpace(c.Impresora) == "" {
		c.Impresora = ColaPorDefecto()
	}
	if _, ok := colorModel[c.Tinta]; !ok {
		c.Tinta = "color"
	}
	if _, ok := calidadModel[c.Calidad]; !ok {
		c.Calidad = "normal"
	}
	return c
}

func leerConfig(datos []byte, c *Config) {
	for _, l := range strings.Split(string(datos), "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)
		si := v == "true" || v == "True" || v == "1"
		switch k {
		case "impresora":
			c.Impresora = v
		case "tinta":
			c.Tinta = v
		case "calidad":
			c.Calidad = v
		case "animacion":
			c.Animacion = si
		case "ascii":
			c.ASCII = si
		case "giro":
			c.Giro = v
		case "timeout_soffice":
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				c.TimeoutSoffice = n
			}
		}
	}
}
