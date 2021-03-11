#!/bin/bash
set -exEuo pipefail

# External dependencies:
# - https://github.com/rpm-software-management/createrepo
# - https://github.com/ericchiang/pup

SRCDIR="$1"  # The directory where .rpm files are located
PKGDIR="$2"  # The local package repository working directory

architectures="x86_64"
# TODO: Replace with CDN URL
#repobaseurl="https://dl.k6.io/rpm"
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/rpm"

mkdir -p "$PKGDIR"
cd "$PKGDIR"

echo "Creating RPM package repository..."
for arch in $architectures; do
  mkdir -p "$arch" && cd "$_"
  # Download existing packages via the CDN to avoid S3 egress costs.
  # This requires an index.html ??
  # TODO: Also check their hashes? Or just sync them with s3cmd which does MD5 checks...
  pkgs=$(curl -fsSL "$repobaseurl/" | pup 'tr.file a attr{href}' | { grep '\.rpm$' || true; })
  echo "$pkgs" | xargs -r -I{} -n1 -P"$(nproc)" curl -fsSLO "$repobaseurl/{}"

  # Copy the new packages in
  find "$SRCDIR" -name "*$arch*.rpm" -type f -print0 | xargs -r0 cp -t .
  createrepo .
  cd -
done

# TODO: sign with gpg
