#!/bin/bash
set -exEuo pipefail

# External dependencies:
# - https://github.com/ericchiang/pup

# TODO: Replace with CDN URL
url='http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/deb/'
files="$(curl -fsSL "$url" | pup 'tr.file a attr{href}' | grep '\.deb$' | sort)"

tmpdir="dist"
mkdir -p "$tmpdir/dists/stable/main/binary-amd64"
existing="$(find "$tmpdir" -maxdepth 1 -name '*.deb' -printf '%P\n' | sort)"
missing=$(comm -3 <(echo "$files") <(echo "$existing"))
echo "$missing" | xargs -I{} -n1 -P"$(nproc)" curl -fsSL -o "dist/{}" "$url{}"
