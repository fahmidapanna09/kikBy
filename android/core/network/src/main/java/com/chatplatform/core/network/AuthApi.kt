package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path

data class RequestOtpBody(val phone_number: String, val purpose: String = "login")
data class VerifyOtpBody(
    val phone_number: String,
    val code: String,
    val device_id: String,
    val platform: String = "android",
    val push_token: String? = null,
    val app_version: String? = null,
)
/** E-mail + password sign-in (the primary flow; phone OTP above is kept but no longer the entry point). */
data class LoginBody(
    val identifier: String, // e-mail or username
    val password: String,
    val device_id: String,
    val platform: String = "android",
    val app_version: String? = null,
)
data class RegisterBody(
    val name: String,
    val username: String,
    val email: String,
    val password: String,
    val device_id: String,
    val platform: String = "android",
    val app_version: String? = null,
)
data class ForgotPasswordBody(val email: String)
data class RefreshBody(val refresh_token: String)

data class TokenPairDto(
    val access_token: String,
    val refresh_token: String,
    val access_expires_at: String,
    val refresh_expires_at: String,
)
// phone_number is absent for e-mail accounts; email is absent for legacy phone accounts.
data class PublicUserDto(
    val id: String,
    val phone_number: String? = null,
    val username: String? = null,
    val email: String? = null,
    val email_verified: Boolean? = null,
)
data class AuthResponseDto(val user: PublicUserDto, val tokens: TokenPairDto, val is_new_user: Boolean)
data class SessionInfoDto(
    val id: String,
    val platform: String,
    val ip_address: String,
    val user_agent: String,
    val issued_at: String,
    val is_current: Boolean,
)
data class SessionsResponseDto(val sessions: List<SessionInfoDto>)

interface AuthApi {
    @POST("v1/auth/otp/request")
    suspend fun requestOtp(@Body body: RequestOtpBody)

    @POST("v1/auth/otp/verify")
    suspend fun verifyOtp(@Body body: VerifyOtpBody): AuthResponseDto

    @POST("v1/auth/login")
    suspend fun login(@Body body: LoginBody): AuthResponseDto

    @POST("v1/auth/register")
    suspend fun register(@Body body: RegisterBody): AuthResponseDto

    @POST("v1/auth/forgot-password")
    suspend fun forgotPassword(@Body body: ForgotPasswordBody)

    @POST("v1/auth/refresh")
    suspend fun refresh(@Body body: RefreshBody): TokenPairDto

    @POST("v1/auth/logout")
    suspend fun logout(@Body body: RefreshBody)

    @GET("v1/auth/sessions")
    suspend fun listSessions(): SessionsResponseDto

    @DELETE("v1/auth/sessions/{sessionId}")
    suspend fun revokeSession(@Path("sessionId") sessionId: String)

    // Phase 10: "log out everywhere" / "I think my account is compromised".
    @POST("v1/auth/sessions/revoke-all")
    suspend fun revokeAllSessions()
}
