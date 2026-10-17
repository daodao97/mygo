#!/bin/bash
# 构建 FreeType + HarfBuzz 静态库（macOS 本机 / Android arm64）
#
# 用法: bash build.sh [macos|android]
#
# 产出在 lib/<target>/{include,lib}，这些是构建产物，被 .gitignore 挡住。
# 想给别人用（CI）就把 lib/<target> 打成 tar.gz 挂到 release 上——
# scripts/android-build.sh 会从那里下载解开。
set -euo pipefail
cd "$(dirname "$0")"

TARGET="${1:-macos}"
ROOT="$(pwd)"
SRC=src
OUT=lib/$TARGET
mkdir -p "$OUT"

# NDK 位置：显式变量优先，其次 ANDROID_HOME/ANDROID_SDK_ROOT，最后按平台猜。
# 不写死某一台的路径——CI 上的 SDK 在 /usr/local/lib/android/sdk。
case "$TARGET" in
  macos)
    CC="clang -arch arm64"
    FT_CONF="--host=aarch64-apple-darwin"
    ;;
  android)
    if [ -z "${NDK:-}" ]; then
      SDK="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
      if [ -z "$SDK" ]; then
        case "$(uname -s)" in
          Darwin) SDK="$HOME/Library/Android/sdk" ;;
          *)      SDK="/usr/local/lib/android/sdk" ;;
        esac
      fi
      NDK="$SDK/ndk/${ANDROID_NDK_VERSION:-28.2.13676358}"
    fi
    case "$(uname -s)" in
      Darwin) HOST_TAG=darwin-x86_64 ;;
      *)      HOST_TAG=linux-x86_64 ;;
    esac
    TOOLBIN="$NDK/toolchains/llvm/prebuilt/$HOST_TAG/bin"
    if [ ! -d "$TOOLBIN" ]; then
      echo "找不到 NDK 工具链：$TOOLBIN" >&2
      echo "  → 设 NDK=/path/to/ndk 或 ANDROID_HOME=/path/to/sdk" >&2
      exit 2
    fi
    CC="$TOOLBIN/aarch64-linux-android29-clang"
    FT_CONF="--host=aarch64-linux-android"

    # 交叉编译配置里的编译器路径随机器变，所以从模板生成，不写死。
    sed -e "s|@CC@|$CC|" \
        -e "s|@CXX@|$TOOLBIN/aarch64-linux-android29-clang++|" \
        -e "s|@AR@|$TOOLBIN/llvm-ar|" \
        -e "s|@STRIP@|$TOOLBIN/llvm-strip|" \
        android-arm64.ini.in > android-arm64.ini
    ;;
esac

# 源码：freetype 与 harfbuzz 的 tarball 不随仓库走（几十 MB），
# 第一次跑时按需下载。
fetch() {
  local name="$1" url="$2" dir="$3"
  if [ -d "$SRC/$dir" ]; then return 0; fi
  mkdir -p "$SRC"
  echo "下载 $name..."
  curl -fsSL "$url" -o "$SRC/$name.tar.gz"
  tar xf "$SRC/$name.tar.gz" -C "$SRC"
  [ -d "$SRC/$name" ] && [ "$name" != "$dir" ] && mv "$SRC/$name" "$SRC/$dir"
  rm -f "$SRC/$name.tar.gz"
}
FT_DIR=freetype-2.13.3
HB_DIR=harfbuzz-10.2.0
fetch "$FT_DIR" "https://download.savannah.gnu.org/releases/freetype/$FT_DIR.tar.gz" "$FT_DIR"
fetch "$HB_DIR" "https://github.com/harfbuzz/harfbuzz/releases/download/10.2.0/$HB_DIR.tar.xz" "$HB_DIR"

# FreeType：最小依赖静态库
if [ ! -f "$OUT/libfreetype.a" ]; then
  (cd "$SRC/$FT_DIR" && \
   CC="$CC" ./configure --prefix="$ROOT/$OUT" --disable-shared \
     --without-brotli --without-bzip2 --without-png --without-zlib --without-harfbuzz \
     $FT_CONF > /dev/null 2>&1 && \
   make -j8 > /dev/null 2>&1 && make install > /dev/null 2>&1) && echo "freetype OK"
fi

# HarfBuzz：禁 glib/icu/cairo，freetype 可选（不用，shaping 自足）
if [ ! -f "$OUT/libharfbuzz.a" ]; then
  (cd "$SRC/$HB_DIR" && \
   meson setup build-$TARGET --cross-file="$ROOT/android-arm64.ini" --prefix="$ROOT/$OUT" \
     -Dglib=disabled -Dicu=disabled -Dcairo=disabled -Dfreetype=disabled \
     -Dtests=disabled -Ddocs=disabled -Dbenchmark=disabled -Dintrospection=disabled \
     --default-library=static --buildtype=release > /dev/null 2>&1 && \
   ninja -C build-$TARGET install > /dev/null 2>&1) && echo "harfbuzz OK"
fi
ls -la $OUT/*.a 2>/dev/null
