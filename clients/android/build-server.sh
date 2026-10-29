#!/bin/sh
# Builds the VodMax server for the app ("Usa questo dispositivo da solo")
# into app/src/main/jniLibs, one per Android architecture. Needs Go and
# the Android NDK (sdk.dir in local.properties, or ANDROID_HOME).
set -e
cd "$(dirname "$0")"
SDK=${ANDROID_HOME:-$(sed -n 's/^sdk.dir=//p' local.properties)}
NDK=$(ls -d "$SDK"/ndk/* | sort -V | tail -1)
CC_DIR=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin
for target in arm64:arm64-v8a:aarch64-linux-android24 arm:armeabi-v7a:armv7a-linux-androideabi24 amd64:x86_64:x86_64-linux-android24; do
  arch=${target%%:*}; rest=${target#*:}; abi=${rest%%:*}; cc=${rest#*:}
  mkdir -p app/src/main/jniLibs/$abi
  # cgo: the system resolver and Android's own linker.
  (cd ../.. && CGO_ENABLED=1 GOOS=android GOARCH=$arch GOARM=7 CC=$CC_DIR/$cc-clang \
    go build -trimpath -ldflags="-s -w" -o clients/android/app/src/main/jniLibs/$abi/libvodmax.so ./cmd/vodmax-server)
  echo "$abi: $(du -h app/src/main/jniLibs/$abi/libvodmax.so | cut -f1)"
done
