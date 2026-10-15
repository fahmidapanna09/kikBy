package com.chatplatform.feature.stories

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.stories.StoriesRepository
import com.chatplatform.domain.stories.StoryViewer

/** Simple by design: fetches once when opened rather than through a
 *  ViewModel, since it's a short-lived read-only detail screen. */
@Composable
fun StoryViewersScreen(repository: StoriesRepository, storyId: String) {
    var viewers by remember { mutableStateOf<List<StoryViewer>?>(null) }
    var error by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(storyId) {
        try {
            viewers = repository.viewers(storyId)
        } catch (e: Exception) {
            error = "Couldn't load viewers."
        }
    }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("Viewed by", style = MaterialTheme.typography.headlineSmall)
        when {
            error != null -> Text(error!!, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 12.dp))
            viewers == null -> CircularProgressIndicator(modifier = Modifier.padding(top = 24.dp))
            viewers!!.isEmpty() -> Text("No views yet.", modifier = Modifier.padding(top = 12.dp), color = MaterialTheme.colorScheme.outline)
            else -> LazyColumn(modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
                items(viewers!!, key = { it.userId }) { v ->
                    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(v.displayName ?: "@${v.username}", modifier = Modifier.weight(1f))
                        v.reaction?.let { Text(it) }
                    }
                }
            }
        }
    }
}
