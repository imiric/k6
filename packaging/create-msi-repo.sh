#!/bin/bash
set -exEuo pipefail

_usage="Usage: $0 <pkgdir> <repodir>"
PKGDIR="${1?${_usage}}"  # The directory where .msi files are located
REPODIR="${2?${_usage}}" # The package repository working directory

mkdir -p "$REPODIR"

find "$PKGDIR" -name "*.msi" -type f -print0 | xargs -r0 cp -t "$REPODIR"
