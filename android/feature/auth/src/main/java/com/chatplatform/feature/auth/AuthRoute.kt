package com.chatplatform.feature.auth

import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue

/**
 * Wire this into your NavHost as the "auth" graph's entry point; call
 * onAuthenticated() to navigate into the main app shell once
 * AuthScreenState.LoggedIn is reached.
 */
@Composable
fun AuthRoute(viewModel: AuthViewModel, onAuthenticated: () -> Unit) {
    val state by viewModel.uiState.collectAsState()

    when (state.screen) {
        AuthScreenState.EmailLogin -> EmailLoginScreen(viewModel)
        AuthScreenState.EmailRegister -> EmailRegisterScreen(viewModel)
        AuthScreenState.ForgotPassword -> ForgotPasswordScreen(viewModel)
        AuthScreenState.PhoneEntry -> PhoneEntryScreen(viewModel)
        AuthScreenState.OtpEntry -> OtpEntryScreen(viewModel)
        AuthScreenState.LoggedIn -> onAuthenticated()
    }
}
