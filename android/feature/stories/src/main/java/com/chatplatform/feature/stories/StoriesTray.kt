package com.chatplatform.feature.stories

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.stories.StoryFeedEntry

/**
 * [onOpenMine] fires when "My status" is tapped and you already have active
 * stories (open the viewer on yourself); [onAddStory] fires when tapped with
 * none yet, or when the small + is tapped explicitly — wire both to
 * CreateStoryScreen / StoryViewerScreen respectively at the nav layer.
 */
@Composable
fun StoriesTray(
    viewModel: StoriesTrayViewModel,
    onOpenMine: () -> Unit,
    onAddStory: () -> Unit,
    onOpenEntry: (StoryFeedEntry) -> Unit,
) {
    val state by viewModel.state.collectAsState()

    LazyRow(
        modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 12.dp),
    ) {
        item {
            TrayItem(
                label = "My status",
                hasUnseen = false,
                hasAny = state.myStories.isNotEmpty(),
                onClick = { if (state.myStories.isNotEmpty()) onOpenMine() else onAddStory() },
                badge = if (state.myStories.isEmpty()) { { Icon(Icons.Filled.Add, contentDescription = "Add a story", modifier = Modifier.size(16.dp)) } } else null,
            )
        }
        items(state.entries, key = { it.authorId }) { entry ->
            TrayItem(
                label = entry.displayName ?: entry.username,
                hasUnseen = entry.hasUnseen && !entry.muted,
                hasAny = true,
                onClick = { onOpenEntry(entry) },
            )
        }
    }
}

@Composable
private fun TrayItem(label: String, hasUnseen: Boolean, hasAny: Boolean, onClick: () -> Unit, badge: (@Composable () -> Unit)? = null) {
    Column(
        modifier = Modifier.clickable(onClick = onClick),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        val ringColor = if (hasUnseen) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant
        androidx.compose.foundation.layout.Box(
            modifier = Modifier
                .size(58.dp)
                .border(width = if (hasAny) 2.dp else 0.dp, color = ringColor, shape = CircleShape)
                .padding(3.dp)
                .background(MaterialTheme.colorScheme.surfaceVariant, CircleShape),
            contentAlignment = Alignment.Center,
        ) {
            Text(label.take(1).uppercase(), style = MaterialTheme.typography.titleMedium)
            if (badge != null) {
                androidx.compose.foundation.layout.Box(
                    modifier = Modifier
                        .align(Alignment.BottomEnd)
                        .size(20.dp)
                        .background(MaterialTheme.colorScheme.primary, CircleShape),
                    contentAlignment = Alignment.Center,
                ) { badge() }
            }
        }
        Text(label.take(10), style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(top = 2.dp))
    }
}
