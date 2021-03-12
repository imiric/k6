#!/bin/bash
set -exEuo pipefail

_s3bucket="test-dl-k6-io"
_usage="Usage: $0 <pkgdir> <repodir> [s3bucket=${_s3bucket}]"
PKGDIR="${1?${_usage}}"  # The directory where .msi files are located
REPODIR="${2?${_usage}}" # The package repository working directory
S3PATH="${3-${_s3bucket}}/msi"

# TODO: Replace with CDN URL
#repobaseurl="https://dl.k6.io/msi"
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/msi"

mkdir -p "$REPODIR" && cd "$_"

# Download existing packages via the CDN to avoid S3 egress costs.
# For MSIs this is just needed to generate the index correctly.
# TODO: Also check their hashes? Or just sync them with s3cmd which does MD5 checks...
pkgs=$(s3cmd ls "s3://${S3PATH}/" | { grep -oP "(?<=/${S3PATH}/).*\.msi" || true; })
echo "$pkgs" | xargs -r -I{} -n1 -P"$(nproc)" curl -fsSLO "$repobaseurl/{}"
find "$PKGDIR" -name "*.msi" -type f -print0 | xargs -r0 cp -t "$REPODIR"
