package com.chatplatform.feature.channels

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
fun CreateChannelScreen(viewModel: CreateChannelViewModel, onCreated: (channelId: String) -> Unit) {
    val state by viewModel.state.collectAsState()
    LaunchedEffect(state.createdId) { state.createdId?.let(onCreated) }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("New channel", style = MaterialTheme.typography.headlineSmall)
        Text(
            "Only you (and admins you add) can post — everyone else follows and reads.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.outline,
            modifier = Modifier.padding(top = 4.dp, bottom = 12.dp),
        )

        OutlinedTextField(value = state.name, onValueChange = viewModel::onNameChanged, label = { Text("Channel name") }, singleLine = true, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(value = state.description, onValueChange = viewModel::onDescriptionChanged, label = { Text("Description (optional)") }, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
        OutlinedTextField(value = state.category, onValueChange = viewModel::onCategoryChanged, label = { Text("Category (optional)") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))

        Row(modifier = Modifier.fillMaxWidth().padding(top = 16.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text("Public", style = MaterialTheme.typography.bodyLarge)
                Text("Shows up in search; otherwise only followable by invite link", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            }
            Switch(checked = state.isPublic, onCheckedChange = viewModel::onIsPublicChanged)
        }

        state.errorMessage?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp)) }

        Button(onClick = viewModel::create, enabled = !state.isCreating, modifier = Modifier.fillMaxWidth().padding(top = 16.dp)) {
            if (state.isCreating) CircularProgressIndicator(modifier = Modifier.padding(2.dp)) else Text("Create channel")
        }
    }
}
