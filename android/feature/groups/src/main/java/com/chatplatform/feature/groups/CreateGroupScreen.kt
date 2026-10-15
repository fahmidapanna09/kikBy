package com.chatplatform.feature.groups

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/** [pickerViewModel] should be built with your own user id as the only
 *  excluded id. [onCreated] receives the new group's conversation id. */
@Composable
fun CreateGroupScreen(
    viewModel: CreateGroupViewModel,
    pickerViewModel: MemberPickerViewModel,
    onCreated: (conversationId: String) -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val picker by pickerViewModel.state.collectAsState()

    LaunchedEffect(state.createdConversationId) {
        state.createdConversationId?.let(onCreated)
    }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("New group", style = MaterialTheme.typography.headlineSmall)

        OutlinedTextField(
            value = state.name,
            onValueChange = viewModel::onNameChanged,
            label = { Text("Group name") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        )
        OutlinedTextField(
            value = state.description,
            onValueChange = viewModel::onDescriptionChanged,
            label = { Text("Description (optional)") },
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
        )

        Text("Add people", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 16.dp, bottom = 4.dp))
        MemberPicker(pickerViewModel, modifier = Modifier.weight(1f))

        state.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
        }

        Button(
            onClick = { viewModel.create(pickerViewModel.selectedUserIds) },
            enabled = !state.isCreating,
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        ) {
            if (state.isCreating) CircularProgressIndicator(modifier = Modifier.size(20.dp))
            else Text(if (picker.selected.isEmpty()) "Create group" else "Create group (${picker.selected.size})")
        }
    }
}
