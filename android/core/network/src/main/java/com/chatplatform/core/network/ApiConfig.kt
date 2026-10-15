package com.chatplatform.core.network

/**
 * Swap BASE_URL per build variant instead of hardcoding it:
 * - Android emulator hitting a host-machine docker-compose stack: http://10.0.2.2:8080/
 * - Real device / Fly.io / EC2 test deployment: https://<your-test-host>/
 * Wire this from BuildConfig (buildConfigField per flavor) once Gradle
 * flavors are set up; a constant is used here to keep this file self-contained.
 */
object ApiConfig {
    const val BASE_URL = "http://10.0.2.2:8080/"
    const val WS_URL = "ws://10.0.2.2:8080/ws"
    const val CONNECT_TIMEOUT_SECONDS = 10L
    const val READ_TIMEOUT_SECONDS = 30L
}
