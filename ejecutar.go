package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ejecutar corre un comando con timeout, en su propio grupo de procesos (para
// poder matar también a los hijos, F-08) y con LC_ALL=C para parsear salidas.
func ejecutar(ctx context.Context, timeout time.Duration, nombre string, args ...string) (string, string, error) {
	return ejecutarEnv(ctx, timeout, true, nombre, args...)
}

// ejecutarEnv: forzarC=false deja el entorno del usuario (LibreOffice elige
// Carta o A4 según el locale; con LC_ALL=C siempre saldría A4).
func ejecutarEnv(ctx context.Context, timeout time.Duration, forzarC bool, nombre string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, nombre, args...)
	cmd.Env = os.Environ()
	if forzarC {
		cmd.Env = append(cmd.Env, "LC_ALL=C", "LANG=C")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = ErrTiempo
	}
	return out.String(), errb.String(), err
}

var ErrTiempo = errors.New("el comando tardó demasiado y se canceló")

func existe(nombre string) bool { _, err := exec.LookPath(nombre); return err == nil }
