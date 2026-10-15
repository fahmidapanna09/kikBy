package com.chatplatform.feature.ai

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Send
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Snackbar
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.ai.AiMessage
import com.chatplatform.domain.ai.AiRole

@Composable
fun AiChatScreen(viewModel: AiChatViewModel) {
    val state by viewModel.state.collectAsState()
    val listState = rememberLazyListState()

    // Auto-scroll to the bottom whenever a new message lands or the
    // streaming reply grows — mirrors ChatScreen's own auto-scroll-on-new-
    // message behavior for 1:1 chat.
    LaunchedEffect(state.messages.size, state.streamingText) {
        val lastIndex = state.messages.size // the streaming bubble, if any, sits one past the last real message
        if (lastIndex >= 0) listState.animateScrollToItem(lastIndex)
    }

    Scaffold(
        topBar = { TopAppBar(title = { Text(state.title) }) },
        bottomBar = {
            Column {
                state.errorMessage?.let {
                    Snackbar(modifier = Modifier.padding(8.dp), action = { }) { Text(it) }
                }
                InputBar(
                    value = state.draftInput,
                    enabled = !state.isSending,
                    onValueChange = viewModel::onInputChanged,
                    onSend = viewModel::send,
                )
            }
        },
    ) { padding ->
        when {
            state.isLoadingHistory -> Box(modifier = Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
            else -> LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(12.dp),
            ) {
                items(state.messages, key = { it.id }) { msg ->
                    MessageBubble(msg)
                }
                if (state.isSending) {
                    item(key = "streaming") {
                        StreamingBubble(text = state.streamingText)
                    }
                }
            }
        }
    }
}

@Composable
private fun MessageBubble(message: AiMessage) {
    val isUser = message.role == AiRole.USER
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
        horizontalArrangement = if (isUser) Arrangement.End else Arrangement.Start,
    ) {
        Box(
            modifier = Modifier
                .widthIn(max = 320.dp)
                .background(
                    color = if (isUser) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceVariant,
                    shape = RoundedCornerShape(12.dp),
                )
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Column {
                Text(
                    text = message.content.ifBlank { "[empty response]" },
                    style = MaterialTheme.typography.bodyLarge,
                )
                if (message.error != null) {
                    Text(
                        "Failed: ${message.error}",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                } else if (message.finishReason != null && message.finishReason != "STOP") {
                    // e.g. SAFETY, MAX_TOKENS — surfaced plainly, no special UI treatment (see README known gaps)
                    Text(
                        "(${message.finishReason})",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.outline,
                    )
                }
            }
        }
    }
}

@Composable
private fun StreamingBubble(text: String) {
    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp), horizontalArrangement = Arrangement.Start) {
        Box(
            modifier = Modifier
                .widthIn(max = 320.dp)
                .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(12.dp))
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Text(
                text = text.ifEmpty { "…" }, // thinking indicator before the first delta arrives
                style = MaterialTheme.typography.bodyLarge,
            )
        }
    }
}

@Composable
private fun InputBar(value: String, enabled: Boolean, onValueChange: (String) -> Unit, onSend: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier.weight(1f),
            placeholder = { Text("Ask the AI agent…") },
            enabled = enabled,
        )
        IconButton(onClick = onSend, enabled = enabled && value.isNotBlank()) {
            if (enabled) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "Send")
            } else {
                CircularProgressIndicator(modifier = Modifier.padding(4.dp))
            }
        }
    }
}
