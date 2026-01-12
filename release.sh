#!/bin/sh
#
# Create release tarballs/zip-files
#

platforms="
  linux,amd64,,linux_x86_64_static,tar.xz
"

# darwin,arm64,,macos_aarch64_static,tar.gz
# linux,riscv64,,linux_riscv64_static,tar.xz

name=vibe67
version=$(grep -i version main.go | head -1 | cut -d' ' -f5 | cut -d'"' -f1)
echo "Version $version"

export CGO_ENABLED=0

compile_and_compress() {
  goos="$1"
  goarch="$2"
  goarm="$3"
  platform="$4"
  compression="$5"

  echo "Compiling $name.$platform..."

  [ -n "$goarm" ] && GOARM="$goarm" || unset GOARM
  GOOS="$goos" GOARCH="$goarch" go build -mod=vendor -trimpath -ldflags="-s -w" -a -o "$name.$platform" || {
    echo "Error: failed to compile for $platform"
    echo "Platform string: $p"
    echo "Environment variables: GOOS=$goos GOARCH=$goarch GOARM=$goarm"
    exit 1
  }

  echo "Compressing $name-$version.$platform.$compression"
  mkdir "$name-$version-$platform"
  cp vibe67.1 "$name-$version-$platform/"
  gzip "$name-$version-$platform/vibe67.1"
  cp "$name.$platform" "$name-$version-$platform/vibe67"
  cp LICENSE "$name-$version-$platform/"

  case "$compression" in
    tar.xz)
      tar Jcf "$name-$version-$platform.$compression" "$name-$version-$platform"
      ;;
    tar.gz)
      tar zcf "$name-$version-$platform.$compression" "$name-$version-$platform"
      ;;
  esac

  rm -r "$name-$version-$platform"
  rm "$name.$platform"
}

echo 'Compiling...'
while read -r p; do
  [ -z "$p" ] && continue
  IFS=',' read -r goos goarch goarm platform compression <<EOF
$p
EOF
  compile_and_compress "$goos" "$goarch" "$goarm" "$platform" "$compression" &
done <<EOF
$platforms
EOF

wait

mkdir -p release
mv -v $name-$version* release
