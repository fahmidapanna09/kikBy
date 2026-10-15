package com.chatplatform.feature.groups

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * Build [pickerViewModel] with `excludedUserIds = groupInfoViewModel.currentMemberIds`
 * so people already in the group don't appear. [onDone] fires after the
 * server accepts the new members.
 */
@Composable
fun AddMembersScreen(
    pickerViewModel: MemberPickerViewModel,
    groupInfoViewModel: GroupInfoViewModel,
    onDone: () -> Unit,
) {
    val picker by pickerViewModel.state.collectAsState()
    val info by groupInfoViewModel.state.collectAsState()

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("Add members", style = MaterialTheme.typography.headlineSmall, modifier = Modifier.padding(bottom = 8.dp))
        MemberPicker(pickerViewModel, modifier = Modifier.weight(1f))

        info.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
        }

        Button(
            onClick = { groupInfoViewModel.addMembers(pickerViewModel.selectedUserIds, onSuccess = onDone) },
            enabled = picker.selected.isNotEmpty(),
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        ) {
            Text(if (picker.selected.isEmpty()) "Add members" else "Add ${picker.selected.size}")
        }
    }
}
