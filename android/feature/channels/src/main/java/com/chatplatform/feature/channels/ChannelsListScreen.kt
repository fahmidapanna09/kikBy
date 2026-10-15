package com.chatplatform.feature.channels

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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.channels.Channel

@Composable
fun ChannelsListScreen(
    viewModel: ChannelsListViewModel,
    onOpenChannel: (channelId: String) -> Unit,
    onCreateChannel: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    var showJoinDialog by remember { mutableStateOf(false) }

    Scaffold(
        floatingActionButton = {
            FloatingActionButton(onClick = onCreateChannel) { Icon(Icons.Filled.Add, contentDescription = "New channel") }
        },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding).padding(12.dp)) {
            OutlinedTextField(
                value = state.searchQuery, onValueChange = viewModel::onSearchQueryChanged,
                modifier = Modifier.fillMaxWidth(), placeholder = { Text("Search channels") }, singleLine = true,
            )
            TextButton(onClick = { showJoinDialog = true }) { Text("Have an invite link? Follow with a code") }

            state.errorMessage?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }

            if (state.searchQuery.isNotBlank()) {
                Text("Search results", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 8.dp))
                LazyColumn(modifier = Modifier.fillMaxWidth()) {
                    items(state.searchResults, key = { it.id }) { c ->
                        ChannelRow(c, onClick = { onOpenChannel(c.id) }, trailing = {
                            if (!c.isFollowing) TextButton(onClick = { viewModel.follow(c.id) }) { Text("Follow") }
                        })
                    }
                }
            } else {
                Text("Your channels", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 8.dp))
                if (state.isLoading) {
                    Row(modifier = Modifier.fillMaxWidth().padding(top = 24.dp), horizontalArrangement = Arrangement.Center) { CircularProgressIndicator() }
                } else if (state.mine.isEmpty()) {
                    Text("You're not following any channels yet.", color = MaterialTheme.colorScheme.outline, modifier = Modifier.padding(top = 8.dp))
                } else {
                    LazyColumn(modifier = Modifier.fillMaxWidth()) {
                        items(state.mine, key = { it.id }) { c -> ChannelRow(c, onClick = { onOpenChannel(c.id) }) }
                    }
                }
            }
        }
    }

    if (showJoinDialog) {
        FollowByCodeDialog(
            onDismiss = { showJoinDialog = false },
            onFollow = { code -> viewModel.followByInvite(code) { id -> onOpenChannel(id) }; showJoinDialog = false },
        )
    }
}

@Composable
private fun ChannelRow(channel: Channel, onClick: () -> Unit, trailing: (@Composable () -> Unit)? = null) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(vertical = 10.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Column {
            Text(channel.name, style = MaterialTheme.typography.bodyLarge)
            Text("${channel.followerCount} followers", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
        }
        trailing?.invoke()
    }
}

@Composable
private fun FollowByCodeDialog(onDismiss: () -> Unit, onFollow: (code: String) -> Unit) {
    var code by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Follow with invite code") },
        text = { OutlinedTextField(value = code, onValueChange = { code = it }, singleLine = true, modifier = Modifier.fillMaxWidth()) },
        confirmButton = { TextButton(onClick = { onFollow(code.trim()) }, enabled = code.isNotBlank()) { Text("Follow") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
