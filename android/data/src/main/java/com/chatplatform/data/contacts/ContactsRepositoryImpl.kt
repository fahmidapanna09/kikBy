package com.chatplatform.data.contacts

import com.chatplatform.core.network.AddContactBody
import com.chatplatform.core.network.BlockedUserDto
import com.chatplatform.core.network.ContactDto
import com.chatplatform.core.network.ContactsApi
import com.chatplatform.core.network.PublicProfileDto
import com.chatplatform.core.network.ReportUserBody
import com.chatplatform.core.network.SyncBody
import com.chatplatform.core.network.UpdateContactBody
import com.chatplatform.core.network.UsersApi
import com.chatplatform.domain.contacts.BlockedUser
import com.chatplatform.domain.contacts.Contact
import com.chatplatform.domain.contacts.ContactsRepository
import com.chatplatform.domain.contacts.PublicProfile

class ContactsRepositoryImpl(
    private val contactsApi: ContactsApi,
    private val usersApi: UsersApi,
) : ContactsRepository {

    override suspend fun addContact(phoneNumber: String, nickname: String?): Contact =
        contactsApi.addContact(AddContactBody(phoneNumber, nickname)).toDomain()

    override suspend fun listContacts(): List<Contact> =
        (contactsApi.listContacts().contacts ?: emptyList()).map { it.toDomain() }

    override suspend fun updateNickname(contactId: String, nickname: String) {
        contactsApi.updateContact(contactId, UpdateContactBody(nickname))
    }

    override suspend fun deleteContact(contactId: String) {
        contactsApi.deleteContact(contactId)
    }

    override suspend fun syncByHashes(phoneHashes: List<String>): List<PublicProfile> {
        // The backend caps a single sync request (see contacts.maxSyncBatch,
        // currently 1000) — chunk here so a large address book doesn't fail
        // the whole sync in one oversized request.
        val matches = mutableListOf<PublicProfile>()
        phoneHashes.chunked(500).forEach { chunk ->
            val res = contactsApi.sync(SyncBody(chunk))
            res.matches?.forEach { m ->
                matches.add(PublicProfile(m.user_id, m.username, m.display_name, about = null, avatarMediaId = null))
            }
        }
        return matches
    }

    override suspend fun blockUser(userId: String) {
        contactsApi.blockUser(userId)
    }

    override suspend fun unblockUser(userId: String) {
        contactsApi.unblockUser(userId)
    }

    override suspend fun listBlocked(): List<BlockedUser> =
        (contactsApi.listBlocked().blocked ?: emptyList()).map { it.toDomain() }

    override suspend fun searchByUsername(prefix: String): List<PublicProfile> =
        (usersApi.search(username = prefix).users ?: emptyList()).map { it.toDomain() }

    override suspend fun searchByPhone(phoneNumber: String): PublicProfile? =
        usersApi.search(phone = phoneNumber).users?.firstOrNull()?.toDomain()

    override suspend fun getPublicProfile(userId: String): PublicProfile =
        usersApi.getPublicProfile(userId).toDomain()

    override suspend fun reportUser(userId: String, reason: String, contextMessageId: String?) {
        usersApi.reportUser(userId, ReportUserBody(reason, contextMessageId))
    }
}

private fun ContactDto.toDomain() = Contact(
    id = id, contactUserId = contact_user_id, phoneNumber = phone_number, nickname = nickname,
    isRegistered = is_registered, username = username, displayName = display_name,
)

private fun BlockedUserDto.toDomain() = BlockedUser(userId = user_id, username = username, displayName = display_name)

private fun PublicProfileDto.toDomain() = PublicProfile(
    userId = user_id, username = username, displayName = display_name, about = about, avatarMediaId = avatar_media_id,
)
