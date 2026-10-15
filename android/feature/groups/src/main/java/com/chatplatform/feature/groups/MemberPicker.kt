package com.chatplatform.feature.groups

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/** Search box + tickable list. Fill the space it's given (put it in a
 *  `Modifier.weight(1f)` slot when composing with other fields). */
@Composable
fun MemberPicker(viewModel: MemberPickerViewModel, modifier: Modifier = Modifier) {
    val state by viewModel.state.collectAsState()
    val searching = state.query.isNotBlank()
    val rows = if (searching) state.searchResults else state.contacts

    Column(modifier = modifier.fillMaxSize()) {
        OutlinedTextField(
            value = state.query,
            onValueChange = viewModel::onQueryChanged,
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("Search by username") },
            singleLine = true,
        )

        if (state.selected.isNotEmpty()) {
            Text(
                text = "${state.selected.size} selected: " + state.selected.values.joinToString(", "),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.primary,
                maxLines = 2,
                modifier = Modifier.padding(top = 8.dp),
            )
        }

        state.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
        }

        when {
            state.isLoading -> Row(
                modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
                horizontalArrangement = Arrangement.Center,
            ) { CircularProgressIndicator() }

            rows.isEmpty() -> Text(
                text = if (searching) "No one found with that username." else "None of your contacts are on the app yet. Try searching by username.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.outline,
                modifier = Modifier.padding(top = 16.dp),
            )

            else -> LazyColumn(modifier = Modifier.fillMaxWidth().padding(top = 4.dp)) {
                items(rows, key = { it.userId }) { member ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { viewModel.toggle(member) }
                            .padding(vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(checked = member.userId in state.selected, onCheckedChange = { viewModel.toggle(member) })
                        Column(modifier = Modifier.padding(start = 8.dp)) {
                            Text(member.label, style = MaterialTheme.typography.bodyLarge)
                            member.subtitle?.let {
                                Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                            }
                        }
                    }
                }
            }
        }
    }
}
