package com.chatplatform.feature.chat

import android.Manifest
import android.content.pm.PackageManager
import android.media.MediaPlayer
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectTapGestures
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
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Send
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.chatplatform.domain.chat.ChatMessage
import com.chatplatform.domain.chat.ConnectionState
import com.chatplatform.domain.chat.MessageDeliveryStatus
import com.chatplatform.domain.chat.ReactionSummary
import com.chatplatform.domain.media.MediaKind
import kotlinx.coroutines.launch

private val QUICK_REACTIONS = listOf("👍", "❤️", "😂", "😮", "😢", "🙏")

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ChatScreen(viewModel: ChatViewModel, currentUserId: String) {
    val state by viewModel.uiState.collectAsState()
    val context = LocalContext.current
    val voiceRecorder = remember { VoiceRecorder(context) }

    var actionsForMessage by remember { mutableStateOf<ChatMessage?>(null) }
    var forwardingMessage by remember { mutableStateOf<ChatMessage?>(null) }
    val forwardTargets by viewModel.forwardTargets().collectAsState(initial = emptyList())

    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val resolver = context.contentResolver
        val mimeType = resolver.getType(uri) ?: "application/octet-stream"
        val sizeBytes = resolver.openAssetFileDescriptor(uri, "r")?.use { it.length } ?: -1L
        val kind = kindForMime(mimeType)
        viewModel.sendAttachment(
            kind = kind,
            mimeType = mimeType,
            fileName = null,
            sizeBytes = sizeBytes,
            openStream = { resolver.openInputStream(uri) ?: error("could not open $uri") },
        )
    }

    val micPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (!granted) {
            // In a full implementation, surface a rationale/snackbar here
            // explaining that voice messages need microphone access.
        }
    }

    Column(modifier = Modifier.fillMaxSize()) {
        if (state.connectionState != ConnectionState.CONNECTED) {
            ConnectionBanner(state.connectionState)
        }
        state.uploadProgress.values.firstOrNull()?.let { progress ->
            LinearProgressIndicator(progress = progress, modifier = Modifier.fillMaxWidth())
        }
        state.errorMessage?.let { message ->
            Text(
                text = message,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(8.dp),
            )
        }

        LazyColumn(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = 12.dp),
            verticalArrangement = Arrangement.Bottom,
        ) {
            items(state.messages, key = { it.clientTmpId }) { message ->
                val reactions = message.serverId?.let { state.reactionsByMessageId[it] } ?: emptyList()
                MessageBubble(
                    message = message,
                    isMine = message.senderId == currentUserId,
                    reactions = reactions,
                    resolveDownloadUrl = { mediaId -> viewModel.downloadUrlFor(mediaId) },
                    onLongPress = { actionsForMessage = message },
                )
                if (actionsForMessage?.clientTmpId == message.clientTmpId) {
                    MessageActionsRow(
                        onReact = { emoji -> viewModel.react(message, emoji); actionsForMessage = null },
                        onForward = { forwardingMessage = message; actionsForMessage = null },
                        onDismiss = { actionsForMessage = null },
                    )
                }
            }
        }

        Composer(
            draft = state.draft,
            isRecording = state.isRecordingVoice,
            onDraftChanged = viewModel::onDraftChanged,
            onSend = viewModel::sendMessage,
            onPickFile = { filePicker.launch("*/*") },
            onStartRecording = {
                val hasPermission = context.checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED
                if (!hasPermission) {
                    micPermission.launch(Manifest.permission.RECORD_AUDIO)
                    return@Composer
                }
                if (voiceRecorder.start().isSuccess) viewModel.setRecordingState(true)
            },
            onStopRecording = {
                if (!voiceRecorder.isRecording) return@Composer
                val note = voiceRecorder.stop()
                viewModel.setRecordingState(false)
                if (note != null) {
                    viewModel.sendVoiceNote(note.file, note.mimeType, note.durationMs)
                }
            },
        )
    }

    forwardingMessage?.let { message ->
        ForwardDialog(
            targets = forwardTargets,
            onPick = { targetId -> viewModel.forwardMessage(message, targetId); forwardingMessage = null },
            onDismiss = { forwardingMessage = null },
        )
    }
}

private fun kindForMime(mimeType: String): MediaKind = when {
    mimeType.startsWith("image/") -> MediaKind.IMAGE
    mimeType.startsWith("video/") -> MediaKind.VIDEO
    mimeType.startsWith("audio/") -> MediaKind.AUDIO
    else -> MediaKind.DOCUMENT
}

@Composable
private fun ConnectionBanner(state: ConnectionState) {
    val text = when (state) {
        ConnectionState.CONNECTING -> "Connecting…"
        ConnectionState.DISCONNECTED -> "Offline — messages will send when you're back online"
        ConnectionState.CONNECTED -> ""
    }
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .padding(8.dp),
    ) {
        Text(text, style = MaterialTheme.typography.bodySmall, modifier = Modifier.align(Alignment.Center))
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun MessageBubble(
    message: ChatMessage,
    isMine: Boolean,
    reactions: List<ReactionSummary>,
    resolveDownloadUrl: suspend (String) -> String,
    onLongPress: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 4.dp),
            horizontalArrangement = if (isMine) Arrangement.End else Arrangement.Start,
        ) {
            Box(
                modifier = Modifier
                    .background(
                        color = if (isMine) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceVariant,
                        shape = RoundedCornerShape(12.dp),
                    )
                    .combinedClickable(onClick = {}, onLongClick = onLongPress)
                    .padding(horizontal = 12.dp, vertical = 8.dp),
            ) {
                Column {
                    if (message.forwardedFromMessageId != null) {
                        Text("Forwarded", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                    }
                    when (message.type) {
                        "image" -> ImageAttachment(message, resolveDownloadUrl)
                        "voice", "audio" -> VoiceAttachment(message, resolveDownloadUrl)
                        else -> Text(text = message.body ?: "[${message.type}]", style = MaterialTheme.typography.bodyLarge)
                    }
                    if (isMine) {
                        Text(
                            text = statusLabel(message.status),
                            style = MaterialTheme.typography.labelSmall,
                            modifier = Modifier.align(Alignment.End),
                        )
                    }
                }
            }
        }
        if (reactions.isNotEmpty()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 4.dp),
                horizontalArrangement = if (isMine) Arrangement.End else Arrangement.Start,
            ) {
                reactions.forEach { r ->
                    Box(
                        modifier = Modifier
                            .padding(end = 4.dp)
                            .background(MaterialTheme.colorScheme.surface, RoundedCornerShape(8.dp))
                            .padding(horizontal = 6.dp, vertical = 2.dp),
                    ) {
                        Text("${r.emoji} ${r.userIds.size}", style = MaterialTheme.typography.labelSmall)
                    }
                }
            }
        }
    }
}

@Composable
private fun MessageActionsRow(onReact: (String) -> Unit, onForward: () -> Unit, onDismiss: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surface, RoundedCornerShape(20.dp))
            .padding(6.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
    ) {
        QUICK_REACTIONS.forEach { emoji ->
            Text(
                text = emoji,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier
                    .padding(4.dp)
                    .clickable { onReact(emoji) },
            )
        }
        Text(
            text = "Forward",
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier
                .padding(4.dp)
                .clickable { onForward() },
        )
    }
}

@Composable
private fun ForwardDialog(targets: List<ForwardTarget>, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Forward to…") },
        text = {
            Column {
                if (targets.isEmpty()) {
                    Text("No other conversations yet.", style = MaterialTheme.typography.bodyMedium)
                }
                targets.forEach { target ->
                    Text(
                        text = target.label,
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 10.dp)
                            .clickable { onPick(target.conversationId) },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}

@Composable
private fun ImageAttachment(message: ChatMessage, resolveDownloadUrl: suspend (String) -> String) {
    val mediaId = message.attachmentIds.firstOrNull()
    var url by remember(mediaId) { mutableStateOf<String?>(null) }

    LaunchedEffect(mediaId) {
        mediaId?.let { url = runCatching { resolveDownloadUrl(it) }.getOrNull() }
    }

    if (mediaId == null) {
        Text("Image unavailable", style = MaterialTheme.typography.bodySmall)
        return
    }
    if (url == null) {
        Box(modifier = Modifier.size(160.dp), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(modifier = Modifier.size(24.dp))
        }
        return
    }
    AsyncImage(
        model = url,
        contentDescription = "Image message",
        modifier = Modifier
            .size(220.dp)
            .background(MaterialTheme.colorScheme.surface, RoundedCornerShape(8.dp)),
    )
}

@Composable
private fun VoiceAttachment(message: ChatMessage, resolveDownloadUrl: suspend (String) -> String) {
    val mediaId = message.attachmentIds.firstOrNull()
    val scope = rememberCoroutineScope()
    var isPlaying by remember { mutableStateOf(false) }

    Row(verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = {
            val id = mediaId ?: return@IconButton
            scope.launch {
                val url = runCatching { resolveDownloadUrl(id) }.getOrNull() ?: return@launch
                isPlaying = true
                val player = MediaPlayer()
                runCatching {
                    player.setDataSource(url)
                    player.setOnCompletionListener { it.release(); isPlaying = false }
                    // NOTE: blocking prepare() for simplicity — use
                    // prepareAsync() + a completion listener in production
                    // to avoid a UI-thread stall on a slow network.
                    player.prepare()
                    player.start()
                }.onFailure { isPlaying = false }
            }
        }) {
            Icon(Icons.Filled.PlayArrow, contentDescription = "Play voice message")
        }
        Text(if (isPlaying) "Playing…" else "Voice message", style = MaterialTheme.typography.bodyMedium)
    }
}

private fun statusLabel(status: MessageDeliveryStatus): String = when (status) {
    MessageDeliveryStatus.PENDING -> "Sending…"
    MessageDeliveryStatus.SENT -> "Sent"
    MessageDeliveryStatus.DELIVERED -> "Delivered"
    MessageDeliveryStatus.READ -> "Read"
    MessageDeliveryStatus.FAILED -> "Failed — will retry"
}

@Composable
private fun Composer(
    draft: String,
    isRecording: Boolean,
    onDraftChanged: (String) -> Unit,
    onSend: () -> Unit,
    onPickFile: () -> Unit,
    onStartRecording: () -> Unit,
    onStopRecording: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconButton(onClick = onPickFile, enabled = !isRecording) {
            Icon(Icons.Filled.AttachFile, contentDescription = "Attach file")
        }
        OutlinedTextField(
            value = draft,
            onValueChange = onDraftChanged,
            modifier = Modifier.weight(1f),
            placeholder = { Text(if (isRecording) "Recording…" else "Message") },
            enabled = !isRecording,
        )
        if (draft.isBlank()) {
            // Press and hold to record, release to send. A real
            // implementation would add a "slide left to cancel" affordance
            // by tracking drag offset inside detectTapGestures/pointerInput;
            // omitted here to keep this within Phase 3's scope.
            Box(
                modifier = Modifier
                    .size(48.dp)
                    .pointerInput(Unit) {
                        detectTapGestures(
                            onPress = {
                                onStartRecording()
                                tryAwaitRelease()
                                onStopRecording()
                            },
                        )
                    },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    Icons.Filled.Mic,
                    contentDescription = "Hold to record a voice message",
                    tint = if (isRecording) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
                )
            }
        } else {
            IconButton(onClick = onSend) {
                Icon(Icons.Filled.Send, contentDescription = "Send")
            }
        }
    }
}
