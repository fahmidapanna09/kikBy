package com.chatplatform.core.network

import kotlinx.coroutines.runBlocking
import okhttp3.Authenticator
import okhttp3.Interceptor
import okhttp3.Request
import okhttp3.Response
import okhttp3.Route

/** Attaches the current access token to every outgoing request. */
class AuthInterceptor(private val accessTokenProvider: suspend () -> String?) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val token = runBlocking { accessTokenProvider() }
        val request = if (token != null) {
            chain.request().newBuilder().addHeader("Authorization", "Bearer $token").build()
        } else {
            chain.request()
        }
        return chain.proceed(request)
    }
}

/**
 * On a 401, attempts exactly one refresh-and-retry per request chain
 * (responseCount check prevents infinite refresh loops on a truly-dead
 * session). onRefreshFailed should route the user back to the phone-entry
 * screen since it means the refresh token itself was rejected.
 */
class TokenAuthenticator(
    private val refreshTokens: suspend () -> Pair<String, String>?, // returns new (access, refresh) or null on failure
    private val onRefreshFailed: () -> Unit,
) : Authenticator {
    override fun authenticate(route: Route?, response: Response): Request? {
        if (responseCount(response) >= 2) return null // already retried once, give up

        val newTokens = runBlocking { refreshTokens() }
        if (newTokens == null) {
            onRefreshFailed()
            return null
        }
        val (newAccessToken, _) = newTokens
        return response.request.newBuilder()
            .header("Authorization", "Bearer $newAccessToken")
            .build()
    }

    private fun responseCount(response: Response): Int {
        var count = 1
        var prior = response.priorResponse
        while (prior != null) {
            count++
            prior = prior.priorResponse
        }
        return count
    }
}
