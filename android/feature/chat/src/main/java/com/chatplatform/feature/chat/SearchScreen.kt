package com.chatplatform.feature.chat

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Search
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.chat.ChatMessage

/**
 * onResultClick receives the tapped result's conversationId — wire it to
 * navigate into that conversation (jump-to-message within the conversation
 * is a further step: scroll ChatScreen's list to the matching seq, not
 * implemented here since this app has no navigation graph yet — see
 * android/README.md).
 */
@Composable
fun SearchScreen(viewModel: SearchViewModel, onResultClick: (ChatMessage) -> Unit) {
    val state by viewModel.uiState.collectAsState()

    Column(modifier = Modifier.fillMaxSize().padding(12.dp)) {
        OutlinedTextField(
            value = state.query,
            onValueChange = viewModel::onQueryChanged,
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("Search messages") },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            singleLine = true,
        )

        when {
            state.isSearching -> CenteredProgress()
            state.errorMessage != null -> Text(
                state.errorMessage!!,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(top = 16.dp),
            )
            state.query.isNotBlank() && state.results.isEmpty() -> Text(
                "No matches",
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(top = 16.dp),
            )
            else -> LazyColumn(modifier = Modifier.fillMaxWidth()) {
                items(state.results, key = { it.clientTmpId }) { message ->
                    SearchResultRow(message, onClick = { onResultClick(message) })
                }
            }
        }
    }
}

@Composable
private fun CenteredProgress() {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        CircularProgressIndicator()
    }
}

@Composable
private fun SearchResultRow(message: ChatMessage, onClick: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(vertical = 10.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Text(
            text = message.body ?: "[${message.type}]",
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 2,
        )
        Text(
            text = "in conversation ${message.conversationId.take(8)}…", // no conversation/contact naming yet — see README
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.outline,
        )
    }
}
