package com.chatplatform.feature.stories

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.chatplatform.domain.stories.StoryType

/** [onOpenViewers] is only invoked from your own stories (see
 *  isOwnStories on the ViewModel) — wire it to a ViewersScreen. */
@Composable
fun StoryViewerScreen(
    viewModel: StoryViewerViewModel,
    onClose: () -> Unit,
    onOpenViewers: (storyId: String) -> Unit = {},
) {
    val state by viewModel.state.collectAsState()

    LaunchedEffect(state.closed) { if (state.closed) onClose() }

    val story = state.currentStory ?: return
    val entry = state.currentEntry ?: return
    val bg = story.backgroundColor?.let { runCatching { Color(android.graphics.Color.parseColor(it)) }.getOrNull() } ?: Color.Black

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(if (story.type == StoryType.TEXT) bg else Color.Black)
            .pointerInput(story.id) {
                detectTapGestures(
                    onPress = { offset ->
                        viewModel.setPaused(true)
                        val pressStartMs = System.currentTimeMillis()
                        val released = tryAwaitRelease()
                        viewModel.setPaused(false)
                        if (released && System.currentTimeMillis() - pressStartMs < 200) {
                            if (offset.x < size.width * 0.3f) viewModel.previous() else viewModel.next()
                        }
                    },
                )
            },
    ) {
        when (story.type) {
            StoryType.TEXT -> Text(
                text = story.textContent ?: "",
                color = Color.White,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.align(Alignment.Center).padding(32.dp),
            )
            StoryType.IMAGE -> state.mediaUrl?.let {
                AsyncImage(model = it, contentDescription = "Story image", modifier = Modifier.fillMaxSize())
            }
            StoryType.VIDEO -> Text(
                // No video player wired up in this pass — see the stories
                // README note. The progress timer still advances on a fixed
                // duration so the flow is testable end to end.
                text = "Video story (playback not wired up yet)",
                color = Color.White,
                modifier = Modifier.align(Alignment.Center).padding(32.dp),
            )
        }

        Column(modifier = Modifier.fillMaxSize().padding(12.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp), modifier = Modifier.fillMaxWidth()) {
                entry.stories.forEachIndexed { i, _ ->
                    val p = when {
                        i < state.storyIndex -> 1f
                        i == state.storyIndex -> state.progress
                        else -> 0f
                    }
                    LinearProgressIndicator(
                        progress = p,
                        modifier = Modifier.weight(1f).height(2.dp).clip(RoundedCornerShape(2.dp)),
                        color = Color.White,
                        trackColor = Color.White.copy(alpha = 0.3f),
                    )
                }
            }

            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        modifier = Modifier.background(Color.White.copy(alpha = 0.2f), CircleShape).padding(8.dp),
                    ) { Text((entry.displayName ?: entry.username).take(1).uppercase(), color = Color.White) }
                    Text(
                        entry.displayName ?: entry.username,
                        color = Color.White,
                        modifier = Modifier.padding(start = 8.dp),
                        fontWeight = FontWeight.Medium,
                    )
                }
                Row {
                    if (viewModel.isOwnStories) {
                        IconButton(onClick = { viewModel.deleteCurrent(onDeleted = {}) }) {
                            Icon(Icons.Filled.Delete, contentDescription = "Delete story", tint = Color.White)
                        }
                    }
                    IconButton(onClick = onClose) {
                        Icon(Icons.Filled.Close, contentDescription = "Close", tint = Color.White)
                    }
                }
            }

            state.errorMessage?.let {
                Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 8.dp))
            }
        }

        Box(modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth().padding(16.dp)) {
            if (viewModel.isOwnStories) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .pointerInput(story.id) { detectTapGestures(onTap = { onOpenViewers(story.id) }) },
                ) {
                    Icon(Icons.Filled.Visibility, contentDescription = null, tint = Color.White)
                    Text(
                        "${story.viewCount ?: 0} views",
                        color = Color.White,
                        modifier = Modifier.padding(start = 6.dp),
                    )
                }
            } else {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    OutlinedTextField(
                        value = state.replyDraft,
                        onValueChange = viewModel::onReplyDraftChanged,
                        modifier = Modifier.weight(1f),
                        placeholder = { Text("Reply", color = Color.White.copy(alpha = 0.6f)) },
                        singleLine = true,
                    )
                    Text(
                        "🔥",
                        modifier = Modifier
                            .padding(start = 8.dp)
                            .pointerInput(story.id) { detectTapGestures(onTap = { viewModel.react("🔥") }) },
                    )
                }
                if (state.replyDraft.isNotBlank()) {
                    Text(
                        "Send",
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier
                            .padding(top = 4.dp)
                            .pointerInput(story.id) { detectTapGestures(onTap = { viewModel.sendReply() }) },
                    )
                }
            }
        }
    }
}
