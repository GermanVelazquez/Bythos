plugins {
    id("com.android.application")
}

android {
    namespace = "com.bythos.movil"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.bythos.movil"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.18.0")
    implementation("androidx.activity:activity-ktx:1.13.0")
    implementation("androidx.work:work-runtime-ktx:2.12.0")
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    testImplementation("junit:junit:4.13.2")
    // android.jar org.json is a stub on the JVM; tests need the real one.
    testImplementation("org.json:json:20250517")
}
