// MyGo's Android host: the `mygo` library (the bridge to Go, the views,
// the web view, native modules' runtime), `app`, the shell app mygo build
// makes for each app, and the app's native modules' Kotlin.
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "mygo-android"
include(":mygo", ":app")

// The Kotlin of the app's native modules, a Gradle library each, which
// mygo build names (mygoNativeModules: name|directory|the modules it uses,
// by comma).
providers.gradleProperty("mygoNativeModules").orNull?.split(',')?.filter { it.isNotEmpty() }?.forEach {
    val (name, dir) = it.split('|')
    include(":$name")
    project(":$name").projectDir = file(dir)
}
