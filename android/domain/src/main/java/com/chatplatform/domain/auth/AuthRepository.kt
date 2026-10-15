package com.chatplatform.domain.auth

data class AuthUser(val id: String, val phoneNumber: String, val username: String?, val email: String? = null)

data class AuthSession(
    val id: String,
    val platform: String,
    val ipAddress: String,
    val userAgent: String,
    val issuedAtIso: String,
    val isCurrent: Boolean,
)

sealed class AuthResult {
    data class Success(val user: AuthUser, val isNewUser: Boolean) : AuthResult()
    data class InvalidCode(val message: String) : AuthResult()
    data class RateLimited(val message: String) : AuthResult()
    data class NetworkError(val message: String) : AuthResult()
    data class Unknown(val message: String) : AuthResult()
}

/**
 * Pure interface; the `data` module provides the Retrofit-backed
 * implementation so `domain` and `feature` never depend on network types
 * directly.
 */
interface AuthRepository {
    suspend fun requestOtp(phoneNumber: String): AuthResult
    suspend fun verifyOtp(phoneNumber: String, code: String, deviceId: String): AuthResult
    /** E-mail (or username) + password. */
    suspend fun login(identifier: String, password: String, deviceId: String): AuthResult
    suspend fun register(name: String, username: String, email: String, password: String, deviceId: String): AuthResult
    /** Always reports success for a well-formed request (the server never reveals whether the address exists). */
    suspend fun forgotPassword(email: String): AuthResult
    suspend fun logout(): AuthResult
    suspend fun listSessions(): List<AuthSession>
    suspend fun revokeSession(sessionId: String)

    /**
     * "Log out everywhere" (Phase 10 hardening) — revokes every session for
     * this account, including the one this device is using right now. The
     * caller should treat a successful return as "we're about to be logged
     * out" and clear local tokens/navigate to login immediately, rather
     * than waiting for the next request to get rejected.
     */
    suspend fun revokeAllSessions()

    fun isLoggedIn(): kotlinx.coroutines.flow.Flow<Boolean>
}
