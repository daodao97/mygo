// The shell app around a MyGo app. mygo build passes the app's ID, name,
// version, signing key, its Go library for each ABI, its manifest and its
// resources.
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

fun prop(name: String) = providers.gradleProperty(name).orNull

android {
    namespace = "dev.mygo.app"
    compileSdk = 36
    defaultConfig {
        applicationId = prop("mygoApplicationId") ?: "dev.mygo.app"
        minSdk = (prop("mygoMinSdk") ?: "26").toInt()
        targetSdk = 36
        versionCode = (prop("mygoVersionCode") ?: "1").toInt()
        versionName = prop("mygoVersionName") ?: "1.0"
        manifestPlaceholders["mygoAppName"] = prop("mygoAppName") ?: "MyGo"
        manifestPlaceholders["mygoIcon"] = prop("mygoIcon") ?: "@android:drawable/sym_def_app_icon"
    }
    signingConfigs {
        create("release") {
            prop("mygoKeystore")?.let {
                storeFile = file(it)
                storePassword = prop("mygoKeystorePassword")
                keyAlias = prop("mygoKeyAlias")
                keyPassword = prop("mygoKeyPassword") ?: prop("mygoKeystorePassword")
            }
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
            // Go's libraries are stripped; its stack traces need no symbols.
            ndk { debugSymbolLevel = "NONE" }
            signingConfig = if (prop("mygoKeystore") != null) signingConfigs.getByName("release") else signingConfigs.getByName("debug")
        }
    }
    sourceSets {
        getByName("main") {
            prop("mygoJniLibs")?.let { jniLibs.srcDir(it) }
            prop("mygoAssets")?.let { assets.srcDir(it) }
            prop("mygoRes")?.let { res.srcDir(it) }
            prop("mygoManifest")?.let { manifest.srcFile(it) }
        }
    }
    packaging { jniLibs { useLegacyPackaging = false } }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin { jvmToolchain(17) }

dependencies {
    implementation(project(":mygo"))
    // The app's native modules' Kotlin (settings.gradle.kts).
    providers.gradleProperty("mygoNativeModules").orNull?.split(',')?.filter { it.isNotEmpty() }?.forEach {
        implementation(project(":" + it.substringBefore('|')))
    }
}
