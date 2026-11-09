package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var convMu sync.Mutex // F-08d: el perfil de LibreOffice no admite concurrencia

func dirCache() string {
	if d := os.Getenv("PRINT_CACHE"); d != "" {
		return d
	}
	h, _ := os.UserCacheDir()
	return filepath.Join(h, "print")
}

// claveCache: ruta absoluta + mtime + tamaño (spec 9.2).
func claveCache(ruta string) (string, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	s := fmt.Sprintf("%s|%d|%d", abs, st.ModTime().UnixNano(), st.Size())
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8]), nil
}

// Convertir devuelve la ruta de un PDF listo para imprimir. Si ya es PDF lo
// devuelve tal cual; si no, usa LibreOffice con perfil aislado y timeout.
func Convertir(ctx context.Context, ruta string, timeout time.Duration) (string, error) {
	if strings.EqualFold(filepath.Ext(ruta), ".pdf") {
		return ruta, nil
	}
	if !existe("soffice") {
		return "", fmt.Errorf("falta LibreOffice (soffice)")
	}
	clave, err := claveCache(ruta)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dirCache(), clave)
	// F-08g: nombre temporal seguro, sin guiones iniciales ni caracteres raros.
	ext := strings.ToLower(filepath.Ext(ruta))
	origen := filepath.Join(dir, "doc"+ext)
	destino := filepath.Join(dir, "doc.pdf")
	if st, err := os.Stat(destino); err == nil && st.Size() > 0 {
		return destino, nil // caché
	}
	convMu.Lock()
	defer convMu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("no puedo escribir en la caché (%v)", err) // F-08h
	}
	if err := copiar(ruta, origen); err != nil {
		return "", err
	}
	// Dos instancias de print comparten el perfil de LibreOffice; sin un candado
	// entre procesos, la segunda conversión fallaba («no produjo un PDF»).
	soltar, err := bloquearPerfil(ctx, timeout)
	if err != nil {
		return "", err
	}
	defer soltar()
	perfil := "-env:UserInstallation=file://" + filepath.Join(dirCache(), "lo-perfil")
	_, errs, err := ejecutarEnv(ctx, timeout, false, "soffice", perfil, "--headless", "--norestore",
		"--convert-to", "pdf", "--outdir", dir, origen)
	if err == ErrTiempo {
		return "", fmt.Errorf("la conversión con LibreOffice tardó más de %s y se canceló", timeout)
	}
	if st, e := os.Stat(destino); e != nil || st.Size() == 0 { // F-08c
		return "", fmt.Errorf("la conversión no produjo un PDF: %s", strings.TrimSpace(errs))
	}
	return destino, nil
}

func copiar(a, b string) error {
	origen, err := os.Open(a)
	if err != nil {
		return err
	}
	defer origen.Close()
	destino, err := os.OpenFile(b, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destino, origen); err != nil {
		destino.Close()
		return err
	}
	return destino.Close()
}

// LimpiarCache borra lo anterior a 24 h (spec 9.2). Los errores se ignoran.
func LimpiarCache(protegido string) {
	ents, _ := os.ReadDir(dirCache())
	for _, e := range ents {
		if !e.IsDir() || e.Name() == "lo-perfil" {
			continue
		}
		p := filepath.Join(dirCache(), e.Name())
		if protegido != "" && strings.HasPrefix(protegido, p) {
			continue
		}
		if i, err := e.Info(); err == nil && time.Since(i.ModTime()) > 24*time.Hour {
			os.RemoveAll(p)
		}
	}
}

// bloquearPerfil toma un candado de archivo (flock) para que solo un proceso use
// el perfil de LibreOffice a la vez. Espera su turno hasta el tiempo dado.
func bloquearPerfil(ctx context.Context, espera time.Duration) (func(), error) {
	if err := os.MkdirAll(dirCache(), 0o755); err != nil {
		return nil, fmt.Errorf("no puedo escribir en la caché (%v)", err)
	}
	f, err := os.OpenFile(filepath.Join(dirCache(), "lo-perfil.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	limite := time.Now().Add(espera)
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if ctx.Err() != nil || time.Now().After(limite) {
			f.Close()
			return nil, fmt.Errorf("otra conversión está en curso y no terminó a tiempo")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
