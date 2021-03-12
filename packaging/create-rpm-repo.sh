#!/bin/bash
set -exEuo pipefail

# External dependencies:
# - https://github.com/rpm-software-management/createrepo
# - https://github.com/s3tools/s3cmd
#   s3cmd expects AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY to be set in the
#   environment.

_s3bucket="test-dl-k6-io"
_usage="Usage: $0 <pkgdir> <repodir> [s3bucket=${_s3bucket}]"
PKGDIR="${1?${_usage}}"  # The directory where .rpm files are located
REPODIR="${2?${_usage}}" # The package repository working directory
S3PATH="${3-${_s3bucket}}/rpm"

architectures="x86_64"
# TODO: Replace with CDN URL
#repobaseurl="https://dl.k6.io/rpm"
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/rpm"

mkdir -p "$REPODIR"
cd "$REPODIR"

echo "Creating RPM package repository..."
for arch in $architectures; do
  mkdir -p "$arch" && cd "$_"
  # Download existing packages via the CDN to avoid S3 egress costs.
  # TODO: Also check their hashes? Or just sync them with s3cmd which does MD5 checks...
  pkgs=$(s3cmd ls "s3://${S3PATH}/" | { grep -oP "(?<=/${S3PATH}/).*\.rpm" || true; })
  echo "$pkgs" | xargs -r -I{} -n1 -P"$(nproc)" curl -fsSLO "$repobaseurl/{}"

  # Copy the new packages in
  find "$PKGDIR" -name "*$arch*.rpm" -type f -print0 | xargs -r0 cp -t .
  createrepo .
  cd -
done

# TODO: sign with gpg
