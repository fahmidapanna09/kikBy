package com.chatplatform.data.auth

import com.chatplatform.core.data.TokenStore
import com.chatplatform.core.network.AuthApi
import com.chatplatform.core.network.ForgotPasswordBody
import com.chatplatform.core.network.LoginBody
import com.chatplatform.core.network.RegisterBody
import com.chatplatform.core.network.RefreshBody
import com.chatplatform.core.network.RequestOtpBody
import com.chatplatform.core.network.VerifyOtpBody
import com.chatplatform.domain.auth.AuthResult
import com.chatplatform.domain.auth.AuthRepository
import com.chatplatform.domain.auth.AuthSession
import com.chatplatform.domain.auth.AuthUser
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import retrofit2.HttpException
import org.json.JSONObject
import java.io.IOException

class AuthRepositoryImpl(
    private val api: AuthApi,
    private val tokenStore: TokenStore,
) : AuthRepository {

    override suspend fun requestOtp(phoneNumber: String): AuthResult {
        return try {
            api.requestOtp(RequestOtpBody(phone_number = phoneNumber))
            AuthResult.Success(AuthUser("", phoneNumber, null), isNewUser = false) // caller only checks success/failure here
        } catch (e: HttpException) {
            mapHttpError(e)
        } catch (e: IOException) {
            AuthResult.NetworkError("Couldn't reach the server. Check your connection.")
        }
    }

    override suspend fun verifyOtp(phoneNumber: String, code: String, deviceId: String): AuthResult {
        return try {
            val res = api.verifyOtp(
                VerifyOtpBody(phone_number = phoneNumber, code = code, device_id = deviceId)
            )
            tokenStore.save(res.tokens.access_token, res.tokens.refresh_token, res.user.id)
            AuthResult.Success(
                AuthUser(res.user.id, res.user.phone_number, res.user.username),
                isNewUser = res.is_new_user,
            )
        } catch (e: HttpException) {
            mapHttpError(e)
        } catch (e: IOException) {
            AuthResult.NetworkError("Couldn't reach the server. Check your connection.")
        }
    }

    override suspend fun login(identifier: String, password: String, deviceId: String): AuthResult =
        passwordAuth { api.login(LoginBody(identifier = identifier.trim(), password = password, device_id = deviceId)) }

    override suspend fun register(name: String, username: String, email: String, password: String, deviceId: String): AuthResult =
        passwordAuth {
            api.register(
                RegisterBody(
                    name = name.trim(), username = username.trim(), email = email.trim(),
                    password = password, device_id = deviceId,
                )
            )
        }

    override suspend fun forgotPassword(email: String): AuthResult {
        return try {
            api.forgotPassword(ForgotPasswordBody(email.trim()))
            AuthResult.Success(AuthUser("", "", null), isNewUser = false)
        } catch (e: HttpException) {
            mapPasswordAuthError(e)
        } catch (e: IOException) {
            AuthResult.NetworkError("Couldn't reach the server. Check your connection.")
        }
    }

    private suspend fun passwordAuth(call: suspend () -> com.chatplatform.core.network.AuthResponseDto): AuthResult {
        return try {
            val res = call()
            tokenStore.save(res.tokens.access_token, res.tokens.refresh_token, res.user.id)
            AuthResult.Success(
                AuthUser(res.user.id, res.user.phone_number ?: "", res.user.username, res.user.email),
                isNewUser = res.is_new_user,
            )
        } catch (e: HttpException) {
            mapPasswordAuthError(e)
        } catch (e: IOException) {
            AuthResult.NetworkError("Couldn't reach the server. Check your connection.")
        }
    }

    /** Uses the server's own wording (error.message, or the first field message for validation errors). */
    private fun mapPasswordAuthError(e: HttpException): AuthResult {
        val err = try {
            JSONObject(e.response()?.errorBody()?.string() ?: "{}").optJSONObject("error")
        } catch (_: Exception) {
            null
        }
        val fields = err?.optJSONObject("fields")
        val firstField = fields?.keys()?.asSequence()?.firstOrNull()?.let { fields.optString(it) }
        val message = firstField?.takeIf { it.isNotBlank() } ?: err?.optString("message")?.takeIf { it.isNotBlank() }
        return when (e.code()) {
            400, 422 -> AuthResult.InvalidCode(message ?: "Please check the details and try again.")
            401 -> AuthResult.InvalidCode(message ?: "Incorrect e-mail/username or password.")
            403 -> AuthResult.Unknown(message ?: "This account can't sign in.")
            429 -> AuthResult.RateLimited(message ?: "Too many attempts. Wait a bit and try again.")
            else -> AuthResult.Unknown("Something went wrong (${e.code()}).")
        }
    }

    override suspend fun logout(): AuthResult {
        return try {
            tokenStore.currentRefreshToken()?.let { api.logout(RefreshBody(it)) }
            tokenStore.clear()
            AuthResult.Success(AuthUser("", "", null), isNewUser = false)
        } catch (e: Exception) {
            // Logout is best-effort server-side; always clear local state so
            // the user isn't stuck logged in on-device even if the network call fails.
            tokenStore.clear()
            AuthResult.Success(AuthUser("", "", null), isNewUser = false)
        }
    }

    override suspend fun listSessions(): List<AuthSession> =
        api.listSessions().sessions.map {
            AuthSession(it.id, it.platform, it.ip_address, it.user_agent, it.issued_at, it.is_current)
        }

    override suspend fun revokeSession(sessionId: String) {
        api.revokeSession(sessionId)
    }

    override suspend fun revokeAllSessions() {
        api.revokeAllSessions()
        // Mirrors logout()'s reasoning: this device's own session is among
        // the ones just revoked server-side, so clear local tokens now
        // rather than waiting for the next request to 401.
        tokenStore.clear()
    }

    override fun isLoggedIn(): Flow<Boolean> = tokenStore.accessTokenFlow.map { it != null }

    /** Performs the refresh-token-rotation call; used by TokenAuthenticator. */
    suspend fun refreshTokens(): Pair<String, String>? {
        val refreshToken = tokenStore.currentRefreshToken() ?: return null
        return try {
            val newTokens = api.refresh(RefreshBody(refreshToken))
            tokenStore.updateAccessToken(newTokens.access_token, newTokens.refresh_token)
            newTokens.access_token to newTokens.refresh_token
        } catch (e: Exception) {
            tokenStore.clear() // refresh token rejected/reused -> force re-login
            null
        }
    }

    private fun mapHttpError(e: HttpException): AuthResult = when (e.code()) {
        400 -> AuthResult.InvalidCode("Invalid request.")
        401 -> AuthResult.InvalidCode("That code didn't match. Try again.")
        429 -> AuthResult.RateLimited("Too many attempts. Wait a bit and try again.")
        else -> AuthResult.Unknown("Something went wrong (${e.code()}).")
    }
}
