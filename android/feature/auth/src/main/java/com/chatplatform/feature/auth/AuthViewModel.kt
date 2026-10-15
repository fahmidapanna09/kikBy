package com.chatplatform.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.auth.AuthRepository
import com.chatplatform.domain.auth.AuthResult
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.util.UUID

sealed class AuthScreenState {
    data object EmailLogin : AuthScreenState()
    data object EmailRegister : AuthScreenState()
    data object ForgotPassword : AuthScreenState()
    data object PhoneEntry : AuthScreenState()
    data object OtpEntry : AuthScreenState()
    data object LoggedIn : AuthScreenState()
}

data class AuthUiState(
    val screen: AuthScreenState = AuthScreenState.EmailLogin,
    val identifier: String = "",
    val password: String = "",
    val name: String = "",
    val username: String = "",
    val email: String = "",
    val infoMessage: String? = null,
    val phoneNumber: String = "",
    val code: String = "",
    val isLoading: Boolean = false,
    val errorMessage: String? = null,
)

class AuthViewModel(
    private val repository: AuthRepository,
    /** Stable per-install device identifier; generate once and persist it
     *  (e.g. in the same DataStore as tokens) rather than regenerating here —
     *  wired as a constructor param so this ViewModel stays testable. */
    private val deviceId: String = UUID.randomUUID().toString(),
) : ViewModel() {

    private val _uiState = MutableStateFlow(AuthUiState())
    val uiState: StateFlow<AuthUiState> = _uiState.asStateFlow()

    fun onIdentifierChanged(v: String) { _uiState.value = _uiState.value.copy(identifier = v, errorMessage = null) }
    fun onPasswordChanged(v: String) { _uiState.value = _uiState.value.copy(password = v, errorMessage = null) }
    fun onNameChanged(v: String) { _uiState.value = _uiState.value.copy(name = v, errorMessage = null) }
    fun onUsernameChanged(v: String) { _uiState.value = _uiState.value.copy(username = v, errorMessage = null) }
    fun onEmailChanged(v: String) { _uiState.value = _uiState.value.copy(email = v, errorMessage = null) }

    fun showEmailLogin() { _uiState.value = _uiState.value.copy(screen = AuthScreenState.EmailLogin, errorMessage = null, infoMessage = null) }
    fun showEmailRegister() { _uiState.value = _uiState.value.copy(screen = AuthScreenState.EmailRegister, errorMessage = null, infoMessage = null) }
    fun showForgotPassword() { _uiState.value = _uiState.value.copy(screen = AuthScreenState.ForgotPassword, errorMessage = null, infoMessage = null) }

    fun login() {
        val s = _uiState.value
        if (s.identifier.isBlank() || s.password.isEmpty()) {
            _uiState.value = s.copy(errorMessage = "Enter your e-mail (or username) and password.")
            return
        }
        launchAuth { repository.login(s.identifier, s.password, deviceId) }
    }

    fun register() {
        val s = _uiState.value
        if (s.name.isBlank() || s.username.isBlank() || s.email.isBlank() || s.password.isEmpty()) {
            _uiState.value = s.copy(errorMessage = "Fill in every field.")
            return
        }
        launchAuth { repository.register(s.name, s.username, s.email, s.password, deviceId) }
    }

    fun sendResetLink() {
        val s = _uiState.value
        if (!s.email.contains("@")) {
            _uiState.value = s.copy(errorMessage = "Enter a valid e-mail address.")
            return
        }
        viewModelScope.launch {
            _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null, infoMessage = null)
            when (val result = repository.forgotPassword(s.email)) {
                is AuthResult.Success -> _uiState.value = _uiState.value.copy(
                    isLoading = false,
                    infoMessage = "If an account exists for that address, a reset link is on its way.",
                )
                else -> _uiState.value = _uiState.value.copy(isLoading = false, errorMessage = errorMessageFor(result))
            }
        }
    }

    private fun launchAuth(call: suspend () -> AuthResult) {
        viewModelScope.launch {
            _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null)
            when (val result = call()) {
                // never keep the password around once it has been used
                is AuthResult.Success -> _uiState.value = _uiState.value.copy(isLoading = false, password = "", screen = AuthScreenState.LoggedIn)
                else -> _uiState.value = _uiState.value.copy(isLoading = false, errorMessage = errorMessageFor(result))
            }
        }
    }

    fun onPhoneNumberChanged(value: String) {
        _uiState.value = _uiState.value.copy(phoneNumber = value, errorMessage = null)
    }

    fun onCodeChanged(value: String) {
        _uiState.value = _uiState.value.copy(code = value, errorMessage = null)
    }

    fun requestOtp() {
        val phone = _uiState.value.phoneNumber
        if (!isPlausiblePhoneNumber(phone)) {
            _uiState.value = _uiState.value.copy(errorMessage = "Enter a phone number with country code, e.g. +8801712345678")
            return
        }
        viewModelScope.launch {
            _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null)
            when (val result = repository.requestOtp(phone)) {
                is AuthResult.Success -> _uiState.value = _uiState.value.copy(isLoading = false, screen = AuthScreenState.OtpEntry)
                else -> _uiState.value = _uiState.value.copy(isLoading = false, errorMessage = errorMessageFor(result))
            }
        }
    }

    fun verifyOtp() {
        val state = _uiState.value
        viewModelScope.launch {
            _uiState.value = state.copy(isLoading = true, errorMessage = null)
            when (val result = repository.verifyOtp(state.phoneNumber, state.code, deviceId)) {
                is AuthResult.Success -> _uiState.value = _uiState.value.copy(isLoading = false, screen = AuthScreenState.LoggedIn)
                else -> _uiState.value = _uiState.value.copy(isLoading = false, errorMessage = errorMessageFor(result))
            }
        }
    }

    fun backToPhoneEntry() {
        _uiState.value = _uiState.value.copy(screen = AuthScreenState.PhoneEntry, code = "", errorMessage = null)
    }

    private fun isPlausiblePhoneNumber(phone: String): Boolean =
        Regex("^\\+[1-9]\\d{6,14}$").matches(phone)

    private fun errorMessageFor(result: AuthResult): String = when (result) {
        is AuthResult.InvalidCode -> result.message
        is AuthResult.RateLimited -> result.message
        is AuthResult.NetworkError -> result.message
        is AuthResult.Unknown -> result.message
        is AuthResult.Success -> "" // unreachable in error branches
    }
}
