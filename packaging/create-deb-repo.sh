#!/bin/bash
set -exEuo pipefail

# External dependencies:
# - https://salsa.debian.org/apt-team/apt (apt-ftparchive, packaged in apt-utils)

SRCDIR="$1"  # The directory where .deb files are located
PKGDIR="$2"  # The local package repository working directory

# We don't publish i386 packages, but the repo structure is needed for
# compatibility on some systems. See https://unix.stackexchange.com/a/272916 .
architectures="amd64 i386"
# TODO: Replace with CDN URL
#repobaseurl="https://dl.k6.io/deb"
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/deb"

mkdir -p "$PKGDIR"
cd "$PKGDIR"

echo "Creating Debian package repository..."
for arch in $architectures; do
  bindir="dists/stable/main/binary-$arch"
  mkdir -p "$bindir"
  # Download existing packages via the CDN to avoid S3 egress costs. This
  # requires the repository to be bootstrapped with at least an empty Packages
  # file, as we want this to fail to avoid recreating the repo with missing
  # packages. An optimization might be to just append to the Packages file and
  # upload it and the new package only, but updating the index.html would get
  # messy and would be inconsistent with the RPM script which does require all
  # packages to be present because of how createrepo works.
  # TODO: Also check their hashes? Or just sync them with s3cmd which does MD5 checks...
  pkgs=$(curl -fsSL "$repobaseurl/$bindir/Packages" | { grep -oP '(?<=Filename: ).*' || true; })
  echo "$pkgs" | xargs -r -I{} -n1 -P"$(nproc)" curl -fsSL -o "{}" "$repobaseurl/{}"

  # Copy the new packages in
  find "$SRCDIR" -name "*$arch*.deb" -type f -print0 | xargs -r0 cp -t "$bindir"
  apt-ftparchive packages "$bindir" | tee "$bindir/Packages"
  gzip -fk "$bindir/Packages"
  bzip2 -fk "$bindir/Packages"
done

echo "Creating release file..."
apt-ftparchive release \
  -o APT::FTPArchive::Release::Origin="k6" \
  -o APT::FTPArchive::Release::Label="k6" \
  -o APT::FTPArchive::Release::Suite="stable" \
  -o APT::FTPArchive::Release::Codename="stable" \
  -o APT::FTPArchive::Release::Architectures="$architectures" \
  -o APT::FTPArchive::Release::Components="main" \
  -o APT::FTPArchive::Release::Date="$(date -Ru)" \
  "dists/stable" > "dists/stable/Release"

# TODO: sign with gpg
