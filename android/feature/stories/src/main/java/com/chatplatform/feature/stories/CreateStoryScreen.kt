package com.chatplatform.feature.stories

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.media.MediaKind

@Composable
fun CreateStoryScreen(viewModel: CreateStoryViewModel, onPosted: () -> Unit) {
    val state by viewModel.state.collectAsState()
    val context = LocalContext.current

    LaunchedEffect(state.posted) { if (state.posted) onPosted() }

    val mediaPicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val resolver = context.contentResolver
        val mimeType = resolver.getType(uri) ?: "image/jpeg"
        val sizeBytes = resolver.openAssetFileDescriptor(uri, "r")?.use { it.length } ?: -1L
        val kind = if (mimeType.startsWith("video/")) MediaKind.VIDEO else MediaKind.IMAGE
        viewModel.postMedia(kind, mimeType, sizeBytes, caption = null) { resolver.openInputStream(uri) ?: error("could not open $uri") }
    }

    val bg = runCatching { Color(android.graphics.Color.parseColor(state.backgroundColor)) }.getOrDefault(Color.DarkGray)

    Column(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f)
                .background(bg),
            contentAlignment = Alignment.Center,
        ) {
            OutlinedTextField(
                value = state.text,
                onValueChange = viewModel::onTextChanged,
                placeholder = { Text("Type a status", color = Color.White.copy(alpha = 0.7f)) },
                modifier = Modifier.padding(24.dp),
            )
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            STORY_BACKGROUND_COLORS.forEach { hex ->
                val c = runCatching { Color(android.graphics.Color.parseColor(hex)) }.getOrDefault(Color.Gray)
                Box(
                    modifier = Modifier
                        .size(28.dp)
                        .clip(CircleShape)
                        .background(c)
                        .border(
                            width = if (hex == state.backgroundColor) 2.dp else 0.dp,
                            color = MaterialTheme.colorScheme.onSurface,
                            shape = CircleShape,
                        )
                        .clickable { viewModel.onBackgroundChanged(hex) },
                )
            }
        }

        state.uploadProgress?.let { LinearProgressIndicator(progress = it, modifier = Modifier.fillMaxWidth()) }
        state.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(horizontal = 12.dp))
        }

        Row(modifier = Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            TextButton(onClick = { mediaPicker.launch("image/*,video/*") }, enabled = !state.isPosting) {
                Text("Add photo/video instead")
            }
            Button(onClick = viewModel::postText, enabled = !state.isPosting && state.text.isNotBlank()) {
                Text("Post")
            }
        }
    }
}
