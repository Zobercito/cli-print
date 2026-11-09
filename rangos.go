package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var ErrTotalDesconocido = errors.New("necesito saber el total de páginas para un rango abierto")

// RangoFueraError indica que se pidió una página mayor al total (F-10b).
type RangoFueraError struct{ Pedida, Max int }

func (e RangoFueraError) Error() string {
	return fmt.Sprintf("la página %d no existe (el documento tiene %d)", e.Pedida, e.Max)
}

// ParseRangos traduce "1-3, 7, 10-" a una lista ordenada, sin duplicados.
// total <= 0 significa "desconocido": no se valida el máximo y los rangos
// abiertos devuelven ErrTotalDesconocido (F-10c).
func ParseRangos(texto string, total int) ([]int, error) {
	texto = strings.ReplaceAll(texto, " ", "")
	if texto == "" {
		return nil, errors.New("rango vacío")
	}
	vistas := map[int]bool{}
	for _, trozo := range strings.Split(texto, ",") {
		if trozo == "" {
			return nil, errors.New("hay una coma sin número")
		}
		ini, fin, err := parseTrozo(trozo, total)
		if err != nil {
			return nil, err
		}
		for p := ini; p <= fin; p++ {
			vistas[p] = true
		}
	}
	out := make([]int, 0, len(vistas))
	for p := range vistas {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

func parseTrozo(t string, total int) (int, int, error) {
	a, b, esRango := strings.Cut(t, "-")
	ini, err := num(a)
	if err != nil {
		return 0, 0, err
	}
	if !esRango {
		return ini, ini, comprobarMax(ini, total)
	}
	if b == "" { // rango abierto "10-"
		if total <= 0 {
			return 0, 0, ErrTotalDesconocido
		}
		if ini > total {
			return 0, 0, RangoFueraError{ini, total}
		}
		return ini, total, nil
	}
	fin, err := num(b)
	if err != nil {
		return 0, 0, err
	}
	if fin < ini {
		return 0, 0, fmt.Errorf("el rango %d-%d está invertido", ini, fin)
	}
	return ini, fin, comprobarMax(fin, total)
}

func num(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%q no es un número de página válido", s)
	}
	return n, nil
}

func comprobarMax(p, total int) error {
	if total > 0 && p > total {
		return RangoFueraError{p, total}
	}
	return nil
}

// FormatoRangos compacta una lista ordenada: [1 3 4 5 9] -> "1,3-5,9".
func FormatoRangos(pags []int) string {
	var partes []string
	for i := 0; i < len(pags); {
		j := i
		for j+1 < len(pags) && pags[j+1] == pags[j]+1 {
			j++
		}
		if j > i {
			partes = append(partes, fmt.Sprintf("%d-%d", pags[i], pags[j]))
		} else {
			partes = append(partes, strconv.Itoa(pags[i]))
		}
		i = j + 1
	}
	return strings.Join(partes, ",")
}
