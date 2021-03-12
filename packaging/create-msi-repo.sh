#!/bin/bash
set -exEuo pipefail

PKGDIR="$1"  # The directory where .msi files are located
REPODIR="$2" # The package repository working directory

mkdir -p "$REPODIR"

find "$PKGDIR" -name "*.msi" -type f -print0 | xargs -r0 cp -t "$REPODIR"
