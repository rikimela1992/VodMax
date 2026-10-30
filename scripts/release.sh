#!/bin/sh
# Builds the release packages into dist/<version>/:
#   vodmax-server_<v>_linux_{amd64,arm64,armv7}.tar.gz  server + systemd unit
#   VodMax-<v>.apk                                      Android (with the built-in server)
#   app.vodmax.client_<v>_all.ipk                        LG webOS
#   SHA256SUMS
# Needs Go; for the APK the Android SDK and NDK (clients/android/local.properties)
# and a JDK 17 (JAVA_HOME); for the IPK Node.js (npx @webos-tools/cli).
#   scripts/release.sh 0.2.0
set -e
v=${1:?versione, es. 0.2.0}
cd "$(dirname "$0")/.."
out=dist/$v
rm -rf "$out" && mkdir -p "$out"

for t in amd64: arm64: arm:7; do
  arch=${t%%:*}; arm=${t#*:}; name=$arch; [ "$arch" = arm ] && name=armv7
  dir=$(mktemp -d)/vodmax-server_${v}_linux_$name
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch GOARM=$arm go build -trimpath -ldflags="-s -w -X main.version=$v" -o "$dir/vodmax-server" ./cmd/vodmax-server
  cp packaging/vodmax.service packaging/vodmax-update README.md docs/install.md docs/installazione.md "$dir/"
  tar -C "$(dirname "$dir")" -czf "$out/$(basename "$dir").tar.gz" "$(basename "$dir")"
  rm -rf "$(dirname "$dir")"
done

if [ -n "$JAVA_HOME" ]; then
  VERSION=$v clients/android/build-server.sh
  # Signed with the release key when there is one (see app/build.gradle),
  # else with the debug key, for local tests only.
  if [ -f "${VODMAX_SIGNING:-$HOME/.config/vodmax-release/keystore.properties}" ]; then
    (cd clients/android && ./gradlew --no-daemon -q assembleRelease)
    cp clients/android/app/build/outputs/apk/release/app-release.apk "$out/VodMax-$v.apk"
  else
    echo "ATTENZIONE: nessuna chiave di rilascio, APK firmato con la chiave di debug" >&2
    (cd clients/android && ./gradlew --no-daemon -q assembleDebug)
    cp clients/android/app/build/outputs/apk/debug/app-debug.apk "$out/VodMax-$v.apk"
  fi
fi

if command -v npx >/dev/null; then
  npx --yes -p @webos-tools/cli@3.2.6 ares-package clients/webos -o "$out"
fi

(cd "$out" && sha256sum -- * > SHA256SUMS)
ls -l "$out"
