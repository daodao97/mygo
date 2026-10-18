#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="$ROOT/.mygo/ios-native-support-tests"
mkdir -p "$OUT"
MAC_SDK="$(xcrun --sdk macosx --show-sdk-path)"
xcrun --sdk macosx swiftc -swift-version 5 -parse-as-library -target arm64-apple-macos15.0 -sdk "$MAC_SDK" \
  native/ios/MyGoActions.swift native/ios/MyGoSharedStore.swift native/ios/tests/ActionsTest.swift -o "$OUT/actions-test"
"$OUT/actions-test"
SIM_SDK="$(xcrun --sdk iphonesimulator --show-sdk-path)"
xcrun --sdk iphonesimulator swiftc -swift-version 5 -target arm64-apple-ios18.0-simulator -sdk "$SIM_SDK" \
  -typecheck native/ios/MyGoActions.swift native/ios/MyGoSharedStore.swift native/ios/MyGoWidgets.swift
xcrun --sdk iphonesimulator clang -target arm64-apple-ios15.0-simulator -isysroot "$SIM_SDK" \
  -fobjc-arc -fblocks -Werror -fsyntax-only native/ios/background/MyGoBackgroundRefresh.m
