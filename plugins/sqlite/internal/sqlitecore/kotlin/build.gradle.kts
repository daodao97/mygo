// The Kotlin of the native module sqlitecore: an Android library, which mygo
// build adds to the app's Gradle build. The build sets its SDK levels, Java
// and Kotlin, and its dependency on MyGo's host library (dev.mygo), which
// brings coroutines, and its NDK and ABIs, which its C builds for.
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.github.egoist.mygo.plugins.sqlite.internal.sqlitecore"
    defaultConfig {
        externalNativeBuild {
            cmake {
                // SQLite's amalgamation downloads once, for every ABI.
                arguments += "-DSQLITE_DOWNLOADS=" + layout.buildDirectory.dir("sqlite").get().asFile.path
            }
        }
    }
    externalNativeBuild {
        cmake { path = file("src/main/c/CMakeLists.txt") }
    }
}

dependencies {
}
