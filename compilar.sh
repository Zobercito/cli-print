#!/bin/sh
# Compila print y lo instala en ~/.local/bin/print.
set -eu
cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
	echo "Error: Go no está instalado." >&2
	echo "Arch Linux: sudo pacman -S go" >&2
	exit 1
fi

echo "Compilando print…"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o print .

mkdir -p "$HOME/.local/bin"
install -m755 print "$HOME/.local/bin/print"

echo "Instalado en ~/.local/bin/print"
"$HOME/.local/bin/print" --version
