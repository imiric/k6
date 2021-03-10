#!/bin/bash
set -exEuo pipefail

SRCDIR="$1"  # The directory where .deb files are located
PKGDIR="$2"  # The local package repository working directory
S3PATH="${3-test-dl-k6-io}/deb/"

# We don't publish i386 packages, but the repo structure is needed for
# compatibility with some apt clients.
architectures="amd64 i386"
# TODO: Replace with CDN URL
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/deb"

mkdir -p "$PKGDIR"
cd "$PKGDIR"

echo "Creating APT repository..."

for arch in $architectures; do
  bindir="dists/stable/main/binary-$arch"
  mkdir -pv "$bindir"
  curl -fsSL -o "$bindir/Packages" "$repobaseurl/$bindir/Packages" || true
  cp "$SRCDIR/"*"$arch"*".deb" "$bindir" || true
  apt-ftparchive packages "$bindir" >> "$bindir/Packages"
  gzip -fk "$bindir/Packages"
done

echo "Syncing APT repository to S3..."
s3cmd sync . "s3://$S3PATH"


# files="$(grep -oP '(?<=Filename: ).*' "$bindir/Packages")"
# mkdir -p "$repodir/$bindir"
# existing="$(find "$repodir" -maxdepth 1 -name '*.deb' -printf '%P\n' | sort)"
# missing=$(comm -3 <(echo "$files") <(echo "$existing"))
# echo "$missing" | xargs -I{} -n1 -P"$(nproc)" curl -fsSL -o "$repodir/$bindir/{}" "$pkgurl/{}"

# cd "$repodir"
# apt-ftparchive packages . | sed 's,^Filename: \./,Filename: ,' > "$bindir/Packages"
# gzip -fk "$bindir/Packages"
