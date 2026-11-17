#!/bin/sh
# Builds CSQLite.xcframework, the SQLite that the module's Swift links on
# iOS, from the amalgamation that the Zig build pins, with its options, as
# a static library for iOS and its simulator on arm64. Its symbols stay
# hidden in the module's framework, as they do in the Zig build's library,
# so that the system's SQLite, which UIKit loads, never meets it. On a
# Mac, with Xcode:
#
#	./build-xcframework.sh out
#
# writes out/CSQLite.xcframework.zip and prints its checksum, for
# Package.swift, which downloads it from the release native-sqlitecore.
set -eu

version=3530400
sha256=1e71ddf93849c6a6ecf58b827c0692073d2dd7ee40196158068f7b29f422e87d
out=$(mkdir -p "$1" && cd "$1" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL -o "$work/sqlite.zip" "https://www.sqlite.org/2026/sqlite-amalgamation-$version.zip"
echo "$sha256  $work/sqlite.zip" | shasum -a 256 -c -
unzip -q "$work/sqlite.zip" -d "$work"
src="$work/sqlite-amalgamation-$version"

mkdir -p "$work/headers"
cp "$src/sqlite3.h" "$work/headers/"
cat > "$work/headers/module.modulemap" <<'MAP'
module CSQLite {
    header "sqlite3.h"
    export *
}
MAP

set --
for sdk in iphoneos:arm64-apple-ios16.0 iphonesimulator:arm64-apple-ios16.0-simulator; do
	name=${sdk%%:*}
	mkdir -p "$work/$name"
	xcrun --sdk "$name" clang -c -O2 -std=c11 -fvisibility=hidden -target "${sdk#*:}" \
		-DSQLITE_THREADSAFE=1 -DSQLITE_DQS=0 -DSQLITE_DEFAULT_FOREIGN_KEYS=1 \
		-DSQLITE_ENABLE_FTS5=1 -DSQLITE_ENABLE_RTREE=1 -DSQLITE_OMIT_LOAD_EXTENSION=1 \
		-DSQLITE_OMIT_DEPRECATED=1 -DSQLITE_OMIT_SHARED_CACHE=1 \
		-o "$work/$name/sqlite3.o" "$src/sqlite3.c"
	xcrun libtool -static -o "$work/$name/libCSQLite.a" "$work/$name/sqlite3.o"
	set -- "$@" -library "$work/$name/libCSQLite.a" -headers "$work/headers"
done
rm -rf "$out/CSQLite.xcframework" "$out/CSQLite.xcframework.zip"
xcodebuild -create-xcframework "$@" -output "$out/CSQLite.xcframework"
(cd "$out" && zip -qry CSQLite.xcframework.zip CSQLite.xcframework)
swift package compute-checksum "$out/CSQLite.xcframework.zip"
