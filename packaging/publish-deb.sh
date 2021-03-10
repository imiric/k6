#!/bin/bash
set -exEuo pipefail

SRCDIR="$1"  # The directory where .deb files are located
PKGDIR="$2"  # The local package repository working directory
S3PATH="${3-test-dl-k6-io}/deb/"  # S3 bucket name and path to repository

# We don't publish i386 packages, but the repo structure is needed for
# compatibility with some apt clients.
architectures="amd64 i386"
# TODO: Replace with CDN URL (or just copy the Packages file from the bucket...)
repobaseurl="http://test-dl-k6-io.s3-website.eu-north-1.amazonaws.com/deb"

mkdir -p "$PKGDIR"
cd "$PKGDIR"

echo "Creating Debian package repository..."
for arch in $architectures; do
  bindir="dists/stable/main/binary-$arch"
  mkdir -pv "$bindir"
  # TODO: This would be safer if it failed (remove '|| true'), so that we don't
  # accidentally overwrite it with missing old packages. But that would require
  # the bucket to be bootstrapped with at least an empty Packages file.
  curl -fsSL -o "$bindir/Packages" "$repobaseurl/$bindir/Packages" || true
  find "$SRCDIR" -name "*$arch*.deb" -type f -print0 | xargs -r0 cp -t "$bindir"
  apt-ftparchive packages "$bindir" | tee -a "$bindir/Packages"
  gzip -fk "$bindir/Packages"
  # TODO: Also compress with bzip2? The Bintray repo has both .gz and .bz2.
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
gzip -fk "dists/stable/Release"

# TODO: sign with gpg

echo "Syncing Debian package repository to S3..."
# s3cmd sync . "s3://$S3PATH"


# files="$(grep -oP '(?<=Filename: ).*' "$bindir/Packages")"
# mkdir -p "$repodir/$bindir"
# existing="$(find "$repodir" -maxdepth 1 -name '*.deb' -printf '%P\n' | sort)"
# missing=$(comm -3 <(echo "$files") <(echo "$existing"))
# echo "$missing" | xargs -I{} -n1 -P"$(nproc)" curl -fsSL -o "$repodir/$bindir/{}" "$pkgurl/{}"

# cd "$repodir"
# apt-ftparchive packages . | sed 's,^Filename: \./,Filename: ,' > "$bindir/Packages"
# gzip -fk "$bindir/Packages"
