package com.chatplatform.feature.groups

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.groups.GroupMember
import com.chatplatform.domain.groups.MemberRole

@Composable
fun GroupInfoScreen(
    viewModel: GroupInfoViewModel,
    currentUserId: String,
    onAddMembers: () -> Unit,
    onLeft: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val clipboard = LocalClipboardManager.current
    var confirmLeave by remember { mutableStateOf(false) }
    val canManage = state.myRole == MemberRole.OWNER || state.myRole == MemberRole.ADMIN

    LaunchedEffect(state.hasLeft) { if (state.hasLeft) onLeft() }

    val group = state.group
    if (group == null) {
        Column(modifier = Modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            if (state.isLoading) CircularProgressIndicator()
            state.errorMessage?.let { Text(it, color = MaterialTheme.colorScheme.error) }
        }
        return
    }

    LazyColumn(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        item {
            Text(group.name, style = MaterialTheme.typography.headlineSmall)
            group.description?.let { Text(it, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 4.dp)) }
            state.errorMessage?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
            }
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("${group.members.size} members", style = MaterialTheme.typography.titleSmall)
                TextButton(onClick = onAddMembers) { Text("Add members") }
            }
        }

        items(group.members, key = { it.userId }) { member ->
            MemberRow(
                label = state.labels[member.userId] ?: "…",
                member = member,
                isMe = member.userId == currentUserId,
                myRole = state.myRole,
                onMakeAdmin = { viewModel.makeAdmin(member.userId) },
                onRemoveAdmin = { viewModel.removeAdmin(member.userId) },
                onRemove = { viewModel.removeMember(member.userId) },
            )
        }

        if (canManage) {
            item {
                Text("Invite link", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp))
                val code = state.inviteCode
                if (code == null) {
                    OutlinedButton(onClick = viewModel::generateInviteCode, modifier = Modifier.padding(top = 8.dp)) {
                        Text("Create invite code")
                    }
                } else {
                    Text(code, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 8.dp))
                    Row(modifier = Modifier.padding(top = 4.dp)) {
                        TextButton(onClick = { clipboard.setText(AnnotatedString(code)) }) { Text("Copy") }
                        TextButton(onClick = viewModel::revokeInviteCode) { Text("Revoke") }
                    }
                }
            }
        }

        // Owners can't leave yet: the backend has no ownership transfer, so an
        // owner leaving would strand the group without one.
        if (state.myRole != MemberRole.OWNER) {
            item {
                Button(onClick = { confirmLeave = true }, modifier = Modifier.fillMaxWidth().padding(top = 24.dp)) {
                    Text("Leave group")
                }
            }
        }
    }

    if (confirmLeave) {
        AlertDialog(
            onDismissRequest = { confirmLeave = false },
            title = { Text("Leave ${group.name}?") },
            text = { Text("You'll stop getting messages from this group.") },
            confirmButton = { TextButton(onClick = { confirmLeave = false; viewModel.leave() }) { Text("Leave") } },
            dismissButton = { TextButton(onClick = { confirmLeave = false }) { Text("Cancel") } },
        )
    }
}

@Composable
private fun MemberRow(
    label: String,
    member: GroupMember,
    isMe: Boolean,
    myRole: MemberRole,
    onMakeAdmin: () -> Unit,
    onRemoveAdmin: () -> Unit,
    onRemove: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    val iAmAdmin = myRole == MemberRole.OWNER || myRole == MemberRole.ADMIN
    // Mirrors the server's rules (which are what actually enforce them): only
    // owners/admins manage others; nobody manages the owner or themselves
    // here; only the owner can demote an admin.
    val hasMenu = iAmAdmin && !isMe && member.role != MemberRole.OWNER

    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Column {
            Text(if (isMe) "$label (you)" else label, style = MaterialTheme.typography.bodyLarge)
            when (member.role) {
                MemberRole.OWNER -> Text("Owner", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
                MemberRole.ADMIN -> Text("Admin", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
                MemberRole.MEMBER -> Unit
            }
        }
        if (hasMenu) {
            Column {
                IconButton(onClick = { menuOpen = true }) { Icon(Icons.Filled.MoreVert, contentDescription = "Member options") }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    if (member.role == MemberRole.MEMBER) {
                        DropdownMenuItem(text = { Text("Make admin") }, onClick = { menuOpen = false; onMakeAdmin() })
                    }
                    if (member.role == MemberRole.ADMIN && myRole == MemberRole.OWNER) {
                        DropdownMenuItem(text = { Text("Remove as admin") }, onClick = { menuOpen = false; onRemoveAdmin() })
                    }
                    DropdownMenuItem(text = { Text("Remove from group") }, onClick = { menuOpen = false; onRemove() })
                }
            }
        }
    }
}
