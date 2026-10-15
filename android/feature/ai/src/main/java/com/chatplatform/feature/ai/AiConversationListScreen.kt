package com.chatplatform.feature.ai

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.ai.AiConversation

@Composable
fun AiConversationListScreen(
    viewModel: AiConversationListViewModel,
    onOpenConversation: (conversationId: String) -> Unit,
) {
    val state by viewModel.state.collectAsState()

    Scaffold(
        topBar = { TopAppBar(title = { Text("AI Agent") }) },
        floatingActionButton = {
            FloatingActionButton(onClick = { viewModel.createConversation(onOpenConversation) }) {
                Icon(Icons.Filled.Add, contentDescription = "New AI chat")
            }
        },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            state.errorMessage?.let {
                Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp))
            }
            when {
                state.isLoading -> CircularProgressIndicator(modifier = Modifier.padding(24.dp))
                state.conversations.isEmpty() -> Text(
                    "No AI chats yet — tap + to start one. It's a private scratchpad: the agent never sees your other chats unless you paste something into it.",
                    modifier = Modifier.padding(24.dp),
                    style = MaterialTheme.typography.bodyMedium,
                )
                else -> LazyColumn(modifier = Modifier.fillMaxWidth()) {
                    items(state.conversations, key = { it.id }) { conv ->
                        ConversationRow(
                            conversation = conv,
                            onClick = { onOpenConversation(conv.id) },
                            onDelete = { viewModel.delete(conv.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun ConversationRow(conversation: AiConversation, onClick: () -> Unit, onDelete: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Row {
            Icon(Icons.Filled.SmartToy, contentDescription = null, modifier = Modifier.padding(end = 12.dp))
            Column {
                Text(conversation.title, style = MaterialTheme.typography.titleMedium)
                Text(conversation.model, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        IconButton(onClick = onDelete) { Icon(Icons.Filled.Delete, contentDescription = "Delete chat") }
    }
}
