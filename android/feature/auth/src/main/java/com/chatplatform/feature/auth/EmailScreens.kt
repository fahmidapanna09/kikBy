package com.chatplatform.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp

@Composable
private fun AuthColumn(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) { content() }
}

@Composable
private fun Feedback(error: String?, info: String?) {
    error?.let {
        Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
    }
    info?.let {
        Text(it, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
    }
}

@Composable
private fun SubmitButton(label: String, loading: Boolean, onClick: () -> Unit) {
    Button(onClick = onClick, enabled = !loading, modifier = Modifier.fillMaxWidth().padding(top = 24.dp)) {
        if (loading) CircularProgressIndicator(modifier = Modifier.size(20.dp)) else Text(label)
    }
}

@Composable
fun EmailLoginScreen(viewModel: AuthViewModel) {
    val state by viewModel.uiState.collectAsState()
    AuthColumn {
        Text("Welcome back", style = MaterialTheme.typography.headlineSmall)
        Text("Sign in with your e-mail (or username).", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp, bottom = 24.dp))
        OutlinedTextField(
            value = state.identifier, onValueChange = viewModel::onIdentifierChanged,
            label = { Text("E-mail or username") }, singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
            modifier = Modifier.fillMaxWidth(), isError = state.errorMessage != null,
        )
        OutlinedTextField(
            value = state.password, onValueChange = viewModel::onPasswordChanged,
            label = { Text("Password") }, singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp), isError = state.errorMessage != null,
        )
        Feedback(state.errorMessage, state.infoMessage)
        SubmitButton("Sign in", state.isLoading, viewModel::login)
        TextButton(onClick = viewModel::showForgotPassword) { Text("Forgot password?") }
        TextButton(onClick = viewModel::showEmailRegister) { Text("Create an account") }
    }
}

@Composable
fun EmailRegisterScreen(viewModel: AuthViewModel) {
    val state by viewModel.uiState.collectAsState()
    AuthColumn {
        Text("Create your account", style = MaterialTheme.typography.headlineSmall)
        Text("We'll e-mail you a link to confirm your address.", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp, bottom = 24.dp))
        OutlinedTextField(value = state.name, onValueChange = viewModel::onNameChanged, label = { Text("Full name") }, singleLine = true, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(value = state.username, onValueChange = viewModel::onUsernameChanged, label = { Text("Username") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(top = 12.dp))
        OutlinedTextField(
            value = state.email, onValueChange = viewModel::onEmailChanged, label = { Text("E-mail") }, singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email), modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        )
        OutlinedTextField(
            value = state.password, onValueChange = viewModel::onPasswordChanged, label = { Text("Password") }, singleLine = true,
            visualTransformation = PasswordVisualTransformation(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp), isError = state.errorMessage != null,
        )
        Feedback(state.errorMessage, state.infoMessage)
        SubmitButton("Create account", state.isLoading, viewModel::register)
        TextButton(onClick = viewModel::showEmailLogin) { Text("I already have an account") }
    }
}

@Composable
fun ForgotPasswordScreen(viewModel: AuthViewModel) {
    val state by viewModel.uiState.collectAsState()
    AuthColumn {
        Text("Reset your password", style = MaterialTheme.typography.headlineSmall)
        Text("Enter your e-mail and we'll send a reset link.", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp, bottom = 24.dp))
        OutlinedTextField(
            value = state.email, onValueChange = viewModel::onEmailChanged, label = { Text("E-mail") }, singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email), modifier = Modifier.fillMaxWidth(), isError = state.errorMessage != null,
        )
        Feedback(state.errorMessage, state.infoMessage)
        SubmitButton("Send reset link", state.isLoading, viewModel::sendResetLink)
        TextButton(onClick = viewModel::showEmailLogin) { Text("Back to sign in") }
    }
}
