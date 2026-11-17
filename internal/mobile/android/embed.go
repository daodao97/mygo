// Package android holds the Android host, Kotlin and Android's views in a
// Gradle project, which mygo build writes out and builds with the app's
// Go code as a shared library. See internal/mobile for how the two work
// together.
package android

import "embed"

// Files are the host's Gradle project.
//
//go:embed settings.gradle.kts build.gradle.kts gradle.properties gradlew gradle app mygo
var Files embed.FS
