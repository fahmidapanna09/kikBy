package com.chatplatform.feature.communities

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
import androidx.compose.material.icons.filled.Campaign
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
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.communities.CommunityGroupRef
import com.chatplatform.domain.communities.CommunityMember
import com.chatplatform.domain.communities.CommunityRole

@Composable
fun CommunityDetailScreen(
    viewModel: CommunityDetailViewModel,
    currentUserId: String,
    onOpenGroup: (conversationId: String) -> Unit,
    onLeft: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val clipboard = LocalClipboardManager.current
    var confirmLeave by remember { mutableStateOf(false) }
    var showAddGroup by remember { mutableStateOf(false) }
    var showReport by remember { mutableStateOf(false) }
    val canManage = state.myRole == CommunityRole.OWNER || state.myRole == CommunityRole.ADMIN

    LaunchedEffect(state.hasLeft) { if (state.hasLeft) onLeft() }

    val community = state.community
    if (community == null) {
        Column(modifier = Modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            if (state.isLoading) CircularProgressIndicator()
            state.errorMessage?.let { Text(it, color = MaterialTheme.colorScheme.error) }
        }
        return
    }

    LazyColumn(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        item {
            Text(community.name, style = MaterialTheme.typography.headlineSmall)
            community.description?.let { Text(it, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 4.dp)) }
            Text("${community.memberCount} members", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, modifier = Modifier.padding(top = 4.dp))
            state.errorMessage?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 8.dp))
            }
        }

        item {
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Groups", style = MaterialTheme.typography.titleSmall)
                if (canManage) {
                    TextButton(onClick = { viewModel.loadAvailableGroups(); showAddGroup = true }) { Text("Add group") }
                }
            }
        }
        items(community.groups, key = { it.conversationId }) { group ->
            GroupRow(group, canManage, onClick = { onOpenGroup(group.conversationId) }, onRemove = { viewModel.removeGroup(group.conversationId) })
        }

        item {
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 20.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text("Members", style = MaterialTheme.typography.titleSmall)
            }
        }
        items(state.members, key = { it.userId }) { member ->
            CommunityMemberRow(
                label = state.labels[member.userId] ?: "…",
                member = member,
                isMe = member.userId == currentUserId,
                myRole = state.myRole,
                onPromote = { viewModel.promote(member.userId) },
                onDemote = { viewModel.demote(member.userId) },
                onRemove = { viewModel.removeMember(member.userId) },
                onBan = { viewModel.banMember(member.userId) },
            )
        }

        if (canManage) {
            item {
                Text("Invite link", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 20.dp))
                val code = community.inviteCode
                if (code == null) {
                    OutlinedButton(onClick = viewModel::generateInviteCode, modifier = Modifier.padding(top = 8.dp)) { Text("Create invite code") }
                } else {
                    Text(code, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 8.dp))
                    Row(modifier = Modifier.padding(top = 4.dp)) {
                        TextButton(onClick = { clipboard.setText(AnnotatedString(code)) }) { Text("Copy") }
                        TextButton(onClick = viewModel::revokeInviteCode) { Text("Revoke") }
                    }
                }
            }
        }

        item {
            Row(modifier = Modifier.fillMaxWidth().padding(top = 24.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                if (state.myRole != CommunityRole.OWNER) {
                    Button(onClick = { confirmLeave = true }) { Text("Leave community") }
                }
                TextButton(onClick = { showReport = true }) { Text("Report") }
            }
        }
    }

    if (confirmLeave) {
        AlertDialog(
            onDismissRequest = { confirmLeave = false },
            title = { Text("Leave ${community.name}?") },
            confirmButton = { TextButton(onClick = { confirmLeave = false; viewModel.leave() }) { Text("Leave") } },
            dismissButton = { TextButton(onClick = { confirmLeave = false }) { Text("Cancel") } },
            text = { Text("You'll be removed from this community and its announcement group.") },
        )
    }

    if (showAddGroup) {
        AddGroupDialog(
            options = state.availableGroupsToAdd,
            onDismiss = { showAddGroup = false },
            onPickExisting = { id -> viewModel.addExistingGroup(id) { showAddGroup = false } },
            onCreateNew = { name -> viewModel.createGroup(name) { showAddGroup = false } },
        )
    }

    if (showReport) {
        ReportDialog(onDismiss = { showReport = false }, onSubmit = { reason -> viewModel.report(reason); showReport = false })
    }
}

@Composable
private fun GroupRow(group: CommunityGroupRef, canManage: Boolean, onClick: () -> Unit, onRemove: () -> Unit) {
    var menuOpen by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (group.isAnnouncement) {
                Icon(Icons.Filled.Campaign, contentDescription = "Announcement group", modifier = Modifier.padding(end = 6.dp))
            }
            Text(group.name, style = MaterialTheme.typography.bodyLarge)
        }
        if (canManage && !group.isAnnouncement) {
            Column {
                IconButton(onClick = { menuOpen = true }) { Icon(Icons.Filled.MoreVert, contentDescription = "Group options") }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    DropdownMenuItem(text = { Text("Remove from community") }, onClick = { menuOpen = false; onRemove() })
                }
            }
        }
    }
}

@Composable
private fun CommunityMemberRow(
    label: String,
    member: CommunityMember,
    isMe: Boolean,
    myRole: CommunityRole,
    onPromote: () -> Unit,
    onDemote: () -> Unit,
    onRemove: () -> Unit,
    onBan: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    val iAmAdmin = myRole == CommunityRole.OWNER || myRole == CommunityRole.ADMIN
    val hasMenu = iAmAdmin && !isMe && member.role != CommunityRole.OWNER

    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Column {
            Text(if (isMe) "$label (you)" else label, style = MaterialTheme.typography.bodyLarge)
            if (member.role != CommunityRole.MEMBER) {
                Text(member.role.name.lowercase().replaceFirstChar { it.uppercase() }, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
            }
        }
        if (hasMenu) {
            Column {
                IconButton(onClick = { menuOpen = true }) { Icon(Icons.Filled.MoreVert, contentDescription = "Member options") }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    if (member.role == CommunityRole.MEMBER) {
                        DropdownMenuItem(text = { Text("Make admin") }, onClick = { menuOpen = false; onPromote() })
                    }
                    if (member.role == CommunityRole.ADMIN && myRole == CommunityRole.OWNER) {
                        DropdownMenuItem(text = { Text("Remove as admin") }, onClick = { menuOpen = false; onDemote() })
                    }
                    DropdownMenuItem(text = { Text("Remove from community") }, onClick = { menuOpen = false; onRemove() })
                    DropdownMenuItem(text = { Text("Ban") }, onClick = { menuOpen = false; onBan() })
                }
            }
        }
    }
}

@Composable
private fun AddGroupDialog(
    options: List<GroupOption>,
    onDismiss: () -> Unit,
    onPickExisting: (conversationId: String) -> Unit,
    onCreateNew: (name: String) -> Unit,
) {
    var newGroupName by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add a group") },
        text = {
            Column {
                OutlinedTextField(
                    value = newGroupName, onValueChange = { newGroupName = it },
                    label = { Text("Or create a new group") }, singleLine = true, modifier = Modifier.fillMaxWidth(),
                )
                if (options.isNotEmpty()) {
                    Text("Link one of your existing groups instead:", style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
                    options.forEach { opt ->
                        Text(
                            opt.name,
                            modifier = Modifier.fillMaxWidth().clickable { onPickExisting(opt.conversationId) }.padding(vertical = 8.dp),
                        )
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onCreateNew(newGroupName.trim()) }, enabled = newGroupName.isNotBlank()) { Text("Create") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ReportDialog(onDismiss: () -> Unit, onSubmit: (reason: String) -> Unit) {
    var reason by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Report this community") },
        text = {
            OutlinedTextField(value = reason, onValueChange = { reason = it }, label = { Text("Reason") }, modifier = Modifier.fillMaxWidth())
        },
        confirmButton = { TextButton(onClick = { onSubmit(reason.trim()) }, enabled = reason.isNotBlank()) { Text("Submit") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
