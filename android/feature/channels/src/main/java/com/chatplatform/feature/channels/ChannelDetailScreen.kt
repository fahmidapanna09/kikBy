package com.chatplatform.feature.channels

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AttachFile
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Poll
import androidx.compose.material.icons.filled.Send
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.channels.ChannelPost
import com.chatplatform.domain.channels.PostType
import com.chatplatform.domain.media.MediaKind

@Composable
fun ChannelDetailScreen(viewModel: ChannelDetailViewModel, currentUserId: String, onDeleted: () -> Unit) {
    val state by viewModel.state.collectAsState()
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    var showPollDialog by remember { mutableStateOf(false) }
    var showAdminMenu by remember { mutableStateOf(false) }
    var showReport by remember { mutableStateOf(false) }
    var draft by remember { mutableStateOf("") }

    LaunchedEffect(state.deleted) { if (state.deleted) onDeleted() }

    val mediaPicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val resolver = context.contentResolver
        val mimeType = resolver.getType(uri) ?: "image/jpeg"
        val sizeBytes = resolver.openAssetFileDescriptor(uri, "r")?.use { it.length } ?: -1L
        val kind = if (mimeType.startsWith("video/")) MediaKind.VIDEO else MediaKind.IMAGE
        viewModel.postMedia(kind, mimeType, sizeBytes, caption = null) { resolver.openInputStream(uri) ?: error("could not open $uri") }
    }

    val channel = state.channel
    if (channel == null) {
        Column(modifier = Modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            if (state.isLoading) CircularProgressIndicator()
            state.errorMessage?.let { Text(it, color = MaterialTheme.colorScheme.error) }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column {
                Text(channel.name, style = MaterialTheme.typography.headlineSmall)
                Text("${channel.followerCount} followers", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            }
            Row {
                if (!channel.isAdmin) {
                    TextButton(onClick = { if (channel.isFollowing) viewModel.unfollow() else viewModel.follow() }) {
                        Text(if (channel.isFollowing) "Following" else "Follow")
                    }
                }
                IconButton(onClick = { showAdminMenu = true }) { Icon(Icons.Filled.MoreVert, contentDescription = "Options") }
                DropdownMenu(expanded = showAdminMenu, onDismissRequest = { showAdminMenu = false }) {
                    if (channel.isAdmin) {
                        DropdownMenuItem(text = { Text("Invite link") }, onClick = { showAdminMenu = false; viewModel.generateInviteCode() })
                        DropdownMenuItem(text = { Text("Delete channel") }, onClick = { showAdminMenu = false; viewModel.deleteChannel() })
                    } else {
                        DropdownMenuItem(text = { Text("Mute") }, onClick = { showAdminMenu = false; viewModel.setMuted(true) })
                        DropdownMenuItem(text = { Text("Unmute") }, onClick = { showAdminMenu = false; viewModel.setMuted(false) })
                    }
                    DropdownMenuItem(text = { Text("Report") }, onClick = { showAdminMenu = false; showReport = true })
                }
            }
        }

        channel.inviteCode?.let { code ->
            Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("Invite: $code", style = MaterialTheme.typography.labelMedium, modifier = Modifier.weight(1f))
                TextButton(onClick = { clipboard.setText(AnnotatedString(code)) }) { Text("Copy") }
            }
        }

        state.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(horizontal = 16.dp))
        }

        LazyColumn(modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 12.dp)) {
            items(state.posts, key = { it.id }) { post ->
                PostCard(
                    post = post,
                    canManage = channel.isAdmin,
                    onReact = { emoji -> viewModel.react(post.id, emoji) },
                    onVote = { optId -> viewModel.vote(post.id, optId) },
                    onDelete = { viewModel.deletePost(post.id) },
                )
            }
            if (state.nextCursor != null) {
                item {
                    TextButton(onClick = viewModel::loadMore, enabled = !state.isLoadingMore) {
                        Text(if (state.isLoadingMore) "Loading…" else "Load more")
                    }
                }
            }
        }

        if (channel.isAdmin) {
            state.uploadProgress?.let { LinearProgressIndicator(progress = it, modifier = Modifier.fillMaxWidth()) }
            Row(modifier = Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { mediaPicker.launch("image/*,video/*") }, enabled = !state.isPosting) {
                    Icon(Icons.Filled.AttachFile, contentDescription = "Attach photo/video")
                }
                IconButton(onClick = { showPollDialog = true }, enabled = !state.isPosting) {
                    Icon(Icons.Filled.Poll, contentDescription = "Create poll")
                }
                OutlinedTextField(
                    value = draft, onValueChange = { draft = it }, modifier = Modifier.weight(1f),
                    placeholder = { Text("Broadcast a message") }, enabled = !state.isPosting,
                )
                IconButton(
                    onClick = { if (draft.isNotBlank()) { viewModel.postText(draft); draft = "" } },
                    enabled = !state.isPosting && draft.isNotBlank(),
                ) { Icon(Icons.Filled.Send, contentDescription = "Post") }
            }
        }
    }

    if (showPollDialog) {
        PollComposerDialog(
            onDismiss = { showPollDialog = false },
            onCreate = { question, options -> viewModel.postPoll(question, options); showPollDialog = false },
        )
    }
    if (showReport) {
        ReportChannelDialog(onDismiss = { showReport = false }, onSubmit = { reason -> viewModel.report(reason); showReport = false })
    }
}

@Composable
private fun PostCard(post: ChannelPost, canManage: Boolean, onReact: (String) -> Unit, onVote: (String) -> Unit, onDelete: () -> Unit) {
    var menuOpen by remember { mutableStateOf(false) }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp)
            .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(12.dp))
            .padding(12.dp),
    ) {
        Column {
            Row(horizontalArrangement = Arrangement.SpaceBetween, modifier = Modifier.fillMaxWidth()) {
                Box(modifier = Modifier.weight(1f)) {
                    when (post.type) {
                        PostType.TEXT -> Text(post.body ?: "", style = MaterialTheme.typography.bodyLarge)
                        PostType.LINK -> Column {
                            post.body?.let { Text(it, style = MaterialTheme.typography.bodyLarge) }
                            Text(post.linkUrl ?: "", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                        }
                        PostType.POLL -> PollContent(post, onVote)
                        PostType.IMAGE, PostType.VIDEO, PostType.AUDIO, PostType.STICKER -> Column {
                            Text("[${post.type.name.lowercase()} attachment]", style = MaterialTheme.typography.bodyMedium)
                            post.body?.let { Text(it, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 4.dp)) }
                        }
                    }
                }
                if (canManage) {
                    Column {
                        IconButton(onClick = { menuOpen = true }, modifier = Modifier.size(24.dp)) {
                            Icon(Icons.Filled.MoreVert, contentDescription = "Post options", modifier = Modifier.size(18.dp))
                        }
                        DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            DropdownMenuItem(text = { Text("Delete") }, onClick = { menuOpen = false; onDelete() })
                        }
                    }
                }
            }

            Row(modifier = Modifier.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("${post.viewCount} views", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                post.reactions.forEach { r ->
                    Text(
                        "  ${r.emoji} ${r.count}",
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.clickable { onReact(r.emoji) },
                    )
                }
                Text(
                    "  👍",
                    style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.clickable { onReact("👍") }.padding(start = 4.dp),
                )
            }
        }
    }
}

@Composable
private fun PollContent(post: ChannelPost, onVote: (String) -> Unit) {
    Column {
        Text(post.body ?: "", style = MaterialTheme.typography.bodyLarge)
        val totalVotes = post.pollOptions.sumOf { it.votes }.coerceAtLeast(1)
        post.pollOptions.forEach { opt ->
            val pct = (opt.votes * 100 / totalVotes).toInt()
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 6.dp).clickable { onVote(opt.id) },
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text(
                    (if (opt.votedByMe) "✓ " else "") + opt.text,
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (opt.votedByMe) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                )
                Text("$pct% (${opt.votes})", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            }
        }
    }
}

@Composable
private fun PollComposerDialog(onDismiss: () -> Unit, onCreate: (question: String, options: List<String>) -> Unit) {
    var question by remember { mutableStateOf("") }
    var optionsText by remember { mutableStateOf(listOf("", "")) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Create a poll") },
        text = {
            Column {
                OutlinedTextField(value = question, onValueChange = { question = it }, label = { Text("Question") }, modifier = Modifier.fillMaxWidth())
                optionsText.forEachIndexed { i, opt ->
                    OutlinedTextField(
                        value = opt,
                        onValueChange = { v -> optionsText = optionsText.toMutableList().also { it[i] = v } },
                        label = { Text("Option ${i + 1}") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                    )
                }
                if (optionsText.size < 10) {
                    TextButton(onClick = { optionsText = optionsText + "" }) { Text("Add option") }
                }
            }
        },
        confirmButton = {
            val cleaned = optionsText.map { it.trim() }.filter { it.isNotEmpty() }
            TextButton(
                onClick = { onCreate(question.trim(), cleaned) },
                enabled = question.isNotBlank() && cleaned.size >= 2,
            ) { Text("Post") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ReportChannelDialog(onDismiss: () -> Unit, onSubmit: (reason: String) -> Unit) {
    var reason by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Report this channel") },
        text = { OutlinedTextField(value = reason, onValueChange = { reason = it }, label = { Text("Reason") }, modifier = Modifier.fillMaxWidth()) },
        confirmButton = { TextButton(onClick = { onSubmit(reason.trim()) }, enabled = reason.isNotBlank()) { Text("Submit") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
