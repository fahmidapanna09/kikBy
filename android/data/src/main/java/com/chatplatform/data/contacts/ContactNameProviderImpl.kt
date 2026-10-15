package com.chatplatform.data.contacts

import com.chatplatform.domain.contacts.ContactNameProvider
import com.chatplatform.domain.contacts.ContactsRepository
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

class ContactNameProviderImpl(private val contacts: ContactsRepository) : ContactNameProvider {

    private val mutex = Mutex()
    private val cache = mutableMapOf<String, String>()
    private var loadedContacts = false

    override suspend fun labelFor(userId: String): String = mutex.withLock {
        if (!loadedContacts) loadContactsLocked()
        cache[userId]?.let { return@withLock it }

        // Not one of the viewer's saved contacts (e.g. a group member they
        // never added, or someone they found via username search) — fall
        // back to that person's public profile.
        val label = runCatching { contacts.getPublicProfile(userId) }
            .getOrNull()
            ?.let { it.displayName?.takeIf { name -> name.isNotBlank() } ?: "@${it.username}" }
            ?: "User ${userId.take(6)}"
        cache[userId] = label
        label
    }

    override suspend fun refresh() = mutex.withLock {
        cache.clear()
        loadContactsLocked()
    }

    private suspend fun loadContactsLocked() {
        // Saved-contact labels win over public-profile labels, so they're
        // written first and never overwritten by labelFor's fallback path.
        runCatching { contacts.listContacts() }.getOrNull()?.forEach { c ->
            c.contactUserId?.let { cache[it] = c.displayLabel }
        }
        loadedContacts = true
    }
}
