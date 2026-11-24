plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val appVersion = (project.findProperty("appVersion") as String?) ?: "1.0.0"
val appCode = (project.findProperty("appCode") as String?)?.toInt() ?: 1

android {
    namespace = "app.infinito.tui"
    compileSdk = 34

    defaultConfig {
        applicationId = "app.infinito.tui"
        minSdk = 24
        // 28: Android permite ejecutar Node.js desde la carpeta de la app
        // (igual que hace Termux). Por eso el APK no puede ir a Google Play.
        @Suppress("ExpiredTargetSdkVersion")
        targetSdk = 28
        versionCode = appCode
        versionName = appVersion
        // Solo las arquitecturas para las que va el binario de Infinito.
        ndk { abiFilters += ((project.findProperty("abis") as String?) ?: "arm64-v8a").split(",") }
    }

    signingConfigs {
        create("release") {
            val ks = System.getenv("ANDROID_KEYSTORE_FILE")
            if (ks != null && file(ks).exists()) {
                storeFile = file(ks)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS") ?: "infinito"
                keyPassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            val rel = signingConfigs.getByName("release")
            signingConfig = if (rel.storeFile != null) rel else signingConfigs.getByName("debug")
        }
    }

    packaging {
        // El binario va como "librería" para poder ejecutarlo; tiene que
        // descomprimirse al instalar.
        jniLibs.useLegacyPackaging = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
        disable += setOf("ExpiredTargetSdkVersion", "OldTargetApi")
    }
}

dependencies {
    implementation("com.github.termux.termux-app:terminal-view:v0.118.3")
}
