#!/bin/bash
set -exEuo pipefail

# External dependencies:
# - https://salsa.debian.org/apt-team/apt (apt-ftparchive, packaged in apt-utils)
# - https://github.com/s3tools/s3cmd
#   s3cmd expects AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY to be set in the
#   environment.

PKGDIR="$1"  # The directory where .deb files are located
REPODIR="$2" # The package repository working directory
S3PATH="${3-test-dl-k6-io}/deb"

# We don't publish i386 packages, but the repo structure is needed for
# compatibility on some systems. See https://unix.stackexchange.com/a/272916 .
architectures="amd64 i386"
# TODO: Replace with CDN URL
#repobaseurl="https://dl.k6.io/deb"
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/deb"

mkdir -p "$REPODIR"
cd "$REPODIR"

echo "Creating Debian package repository..."
for arch in $architectures; do
  bindir="dists/stable/main/binary-$arch"
  mkdir -p "$bindir"
  # Download existing packages via the CDN to avoid S3 egress costs.
  # An optimization might be to just append to the Packages file and upload it
  # and the new package only, but updating the index.html would get messy and
  # would be inconsistent with the RPM script which does require all packages to
  # be present because of how createrepo works.
  # TODO: Also check their hashes? Or just sync them with s3cmd which does MD5 checks...
  pkgs=$(s3cmd ls "s3://${S3PATH}/${bindir}/" | { grep -oP "(?<=/${S3PATH}/).*\.deb" || true; })
  echo "$pkgs" | xargs -r -I{} -n1 -P"$(nproc)" curl -fsSL -o "{}" "$repobaseurl/{}"

  # Copy the new packages in
  find "$PKGDIR" -name "*$arch*.deb" -type f -print0 | xargs -r0 cp -t "$bindir"
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
