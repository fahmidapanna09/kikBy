package com.chatplatform.feature.contacts

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.chatplatform.domain.contacts.Contact

/**
 * onStartChat receives the tapped contact's contact_user_id — wire it to
 * ChatRepository.startConversation() + navigation once this app has a nav
 * graph (see android/README.md — there isn't one yet in this scaffold).
 */
@Composable
fun ContactsScreen(viewModel: ContactsViewModel, onStartChat: (userId: String) -> Unit) {
    val state by viewModel.uiState.collectAsState()
    val context = LocalContext.current
    var showAddDialog by remember { mutableStateOf(false) }

    val contactsPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) {
            val numbers = DeviceContactsReader.readAll(context).map { it.phoneNumber }
            viewModel.syncDeviceContacts(numbers)
        }
    }

    Scaffold(
        floatingActionButton = {
            FloatingActionButton(onClick = { showAddDialog = true }) {
                Icon(Icons.Filled.Add, contentDescription = "Add contact")
            }
        },
    ) { padding ->
        Column(modifier = Modifier
            .fillMaxSize()
            .padding(padding)
            .padding(12.dp)) {

            OutlinedTextField(
                value = state.searchQuery,
                onValueChange = viewModel::onSearchQueryChanged,
                modifier = Modifier.fillMaxWidth(),
                placeholder = { Text("Search by username") },
                singleLine = true,
            )

            TextButton(onClick = {
                val hasPermission = context.checkSelfPermission(Manifest.permission.READ_CONTACTS) == PackageManager.PERMISSION_GRANTED
                if (hasPermission) {
                    val numbers = DeviceContactsReader.readAll(context).map { it.phoneNumber }
                    viewModel.syncDeviceContacts(numbers)
                } else {
                    contactsPermission.launch(Manifest.permission.READ_CONTACTS)
                }
            }) {
                Text(if (state.isSyncing) "Syncing…" else "Sync from device contacts")
            }

            state.errorMessage?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }

            if (state.searchQuery.isNotBlank() && state.searchResults.isNotEmpty()) {
                Text("Search results", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 12.dp))
                state.searchResults.forEach { profile ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { onStartChat(profile.userId) }
                            .padding(vertical = 8.dp),
                    ) {
                        Text("@${profile.username}" + (profile.displayName?.let { " ($it)" } ?: ""))
                    }
                }
            }

            if (state.isLoading) {
                Row(modifier = Modifier.fillMaxWidth().padding(top = 24.dp), horizontalArrangement = Arrangement.Center) {
                    CircularProgressIndicator()
                }
            } else {
                LazyColumn(modifier = Modifier.fillMaxWidth().padding(top = 12.dp)) {
                    items(state.contacts, key = { it.id }) { contact ->
                        ContactRow(
                            contact = contact,
                            onClick = { contact.contactUserId?.let(onStartChat) },
                            onDelete = { viewModel.deleteContact(contact.id) },
                            onBlock = { contact.contactUserId?.let(viewModel::blockContact) },
                        )
                    }
                }
            }
        }
    }

    if (showAddDialog) {
        AddContactDialog(
            onDismiss = { showAddDialog = false },
            onAdd = { phone, nickname -> viewModel.addContact(phone, nickname); showAddDialog = false },
        )
    }
}

@Composable
private fun ContactRow(contact: Contact, onClick: () -> Unit, onDelete: () -> Unit, onBlock: () -> Unit) {
    var menuOpen by remember { mutableStateOf(false) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = contact.isRegistered, onClick = onClick)
            .padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Column {
            Text(contact.displayLabel, style = MaterialTheme.typography.bodyLarge)
            if (!contact.isRegistered) {
                Text("Not on this app yet", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            }
        }
        Row {
            IconButton(onClick = { menuOpen = true }) {
                Icon(Icons.Filled.MoreVert, contentDescription = "More options")
            }
            DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                DropdownMenuItem(text = { Text("Delete") }, onClick = { menuOpen = false; onDelete() })
                if (contact.isRegistered) {
                    DropdownMenuItem(text = { Text("Block") }, onClick = { menuOpen = false; onBlock() })
                }
            }
        }
    }
}

@Composable
private fun AddContactDialog(onDismiss: () -> Unit, onAdd: (phoneNumber: String, nickname: String?) -> Unit) {
    var phone by remember { mutableStateOf("") }
    var nickname by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add contact") },
        text = {
            Column {
                OutlinedTextField(
                    value = phone,
                    onValueChange = { phone = it },
                    label = { Text("Phone number (e.g. +8801712345678)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = nickname,
                    onValueChange = { nickname = it },
                    label = { Text("Nickname (optional)") },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onAdd(phone, nickname.ifBlank { null }) }, enabled = phone.isNotBlank()) {
                Text("Add")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}
