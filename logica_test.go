package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseRangos(t *testing.T) {
	ok := []struct {
		in    string
		total int
		want  []int
	}{
		{"1-3", 14, []int{1, 2, 3}},
		{"1,5,9", 14, []int{1, 5, 9}},
		{"1 - 3 , 7, 12-", 14, []int{1, 2, 3, 7, 12, 13, 14}},
		{"3-5,4-8,1", 14, []int{1, 3, 4, 5, 6, 7, 8}},
		{"2,2", 0, []int{2}},
	}
	for _, c := range ok {
		got, err := ParseRangos(c.in, c.total)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"", "5-2", "0", "a", "1,,3", "-3", "1-2-3"} {
		if _, err := ParseRangos(in, 14); err == nil {
			t.Errorf("%q debería fallar", in)
		}
	}
	if _, err := ParseRangos("1-30", 14); err == nil {
		t.Error("fuera de rango debería fallar")
	} else if _, ok := err.(RangoFueraError); !ok {
		t.Errorf("tipo de error: %T", err)
	}
	if _, err := ParseRangos("10-", 0); err != ErrTotalDesconocido {
		t.Errorf("abierto sin total: %v", err)
	}
}

func TestFormatoRangos(t *testing.T) {
	if g := FormatoRangos([]int{1, 3, 4, 5, 9, 10}); g != "1,3-5,9-10" {
		t.Error(g)
	}
}

func TestPlanDuplex(t *testing.T) {
	rng := func(a, b int) []int {
		var s []int
		for i := a; i <= b; i++ {
			s = append(s, i)
		}
		return s
	}
	casos := []struct {
		sel        []int
		a, r       []int
		hojas, sin int
	}{
		{rng(1, 14), []int{1, 3, 5, 7, 9, 11, 13}, []int{2, 4, 6, 8, 10, 12, 14}, 7, 0},
		{rng(1, 13), []int{1, 3, 5, 7, 9, 11, 13}, []int{2, 4, 6, 8, 10, 12}, 7, 13},
		{rng(3, 8), []int{3, 5, 7}, []int{4, 6, 8}, 3, 0},
		{rng(2, 7), []int{2, 4, 6}, []int{3, 5, 7}, 3, 0}, // anverso par
		{[]int{1, 2, 3, 7, 10, 11, 12}, []int{1, 3, 10, 12}, []int{2, 7, 11}, 4, 12},
	}
	for _, c := range casos {
		p := PlanificarDuplex(c.sel)
		if !reflect.DeepEqual(p.A, c.a) || !reflect.DeepEqual(p.R, c.r) || p.Hojas != c.hojas || p.SinReverso != c.sin {
			t.Errorf("%v: %+v", c.sel, p)
		}
	}
	if DuplexUtil([]int{1}) || !DuplexUtil([]int{1, 2}) {
		t.Error("DuplexUtil")
	}
	if OrientacionReverso("Portrait") != "UpsideDown" || OrientacionReverso("Landscape") != "Seascape" {
		t.Error("orientación")
	}
}

func TestConstruirLP(t *testing.T) {
	got := ConstruirLP(Opciones{Cola: "Canon_G2110", Papel: "A4", Tinta: "color",
		Orientacion: "Portrait", Copias: 2, Paginas: []int{1, 3, 5, 6, 7}}, "-raro; $(x).pdf")
	want := []string{"-d", "Canon_G2110", "-n", "2", "-o", "PageSize=A4", "-o", "ColorModel=CMY",
		"-o", "StpOrientation=Portrait", "-o", "page-ranges=1,3,5-7", "--", "-raro; $(x).pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestEscalaEnLP(t *testing.T) {
	got := ConstruirLP(Opciones{Cola: "c", Escala: "fit"}, "a.pdf")
	want := []string{"-d", "c", "-o", "print-scaling=fit", "--", "a.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestOrdenEnLP(t *testing.T) {
	got := ConstruirLP(Opciones{Cola: "c", Orden: "normal"}, "a.pdf")
	want := []string{"-d", "c", "-o", "outputorder=normal", "--", "a.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := ConstruirLP(Opciones{Cola: "c"}, "a.pdf"); reflect.DeepEqual(got, want) {
		t.Errorf("sin Orden no debe inventarse outputorder: %q", got)
	}
}

func TestParsearArgsFlagsDespuesDelArchivo(t *testing.T) {
	a, err := parsearArgs([]string{"doc.pdf", "--dry-run", "--copias", "3"}, io.Discard)
	if err != nil || !a.dry || a.ruta != "doc.pdf" || a.copias != 3 {
		t.Errorf("%+v %v", a, err)
	}
	a, _ = parsearArgs([]string{"--dry-run", "doc.pdf"}, io.Discard)
	if !a.dry || a.ruta != "doc.pdf" {
		t.Errorf("%+v", a)
	}
	// varios argumentos que no son un archivo existente: error claro
	if _, err := parsearArgs([]string{"mi", "archivo.pdf"}, io.Discard); err == nil {
		t.Error("debería pedir comillas")
	}
	// nombre con espacios sin comillas pero que existe
	f := filepath.Join(t.TempDir(), "mi doc.pdf")
	os.WriteFile(f, []byte("x"), 0o644)
	partes := strings.Fields(f)
	if len(partes) > 1 {
		if a, err := parsearArgs(partes, io.Discard); err != nil || a.ruta != f {
			t.Errorf("%+v %v", a, err)
		}
	}
}

func TestPlanDuplexOrdenDeImpresion(t *testing.T) {
	p := PlanificarDuplex([]int{1, 2, 3, 4})
	if !reflect.DeepEqual(p.Pasada1, []int{4, 2}) || !reflect.DeepEqual(p.Pasada2, []int{3, 1}) {
		t.Errorf("4 páginas: %+v", p)
	}
	// impar: la última hoja no tiene reverso, así que su frente va primero en la pasada 2
	p = PlanificarDuplex([]int{1, 2, 3, 4, 5})
	if !reflect.DeepEqual(p.Pasada1, []int{4, 2}) || !reflect.DeepEqual(p.Pasada2, []int{5, 3, 1}) || p.SinReverso != 5 {
		t.Errorf("5 páginas: %+v", p)
	}
	// rango que empieza en par
	p = PlanificarDuplex([]int{2, 3, 4, 5, 6, 7})
	if !reflect.DeepEqual(p.Pasada1, []int{7, 5, 3}) || !reflect.DeepEqual(p.Pasada2, []int{6, 4, 2}) {
		t.Errorf("rango 2-7: %+v", p)
	}
}
