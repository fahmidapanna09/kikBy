package com.chatplatform.domain.contacts

data class Contact(
    val id: String,
    val contactUserId: String?,
    val phoneNumber: String,
    val nickname: String?,
    val isRegistered: Boolean,
    val username: String?,
    val displayName: String?,
) {
    /** What to show in a contact list row: nickname > registered display
     *  name > username > raw phone number, in that order of preference. */
    val displayLabel: String
        get() = nickname ?: displayName ?: username?.let { "@$it" } ?: phoneNumber
}

data class PublicProfile(
    val userId: String,
    val username: String,
    val displayName: String?,
    val about: String?,
    val avatarMediaId: String?,
)

data class BlockedUser(val userId: String, val username: String, val displayName: String?)

interface ContactsRepository {
    suspend fun addContact(phoneNumber: String, nickname: String?): Contact
    suspend fun listContacts(): List<Contact>
    suspend fun updateNickname(contactId: String, nickname: String)
    suspend fun deleteContact(contactId: String)

    /** phoneHashes must already be SHA-256 hex of each E.164 number — see
     *  PhoneHasher. Returns only contacts that turned out to be registered;
     *  the caller decides whether to also call addContact() for any of them. */
    suspend fun syncByHashes(phoneHashes: List<String>): List<PublicProfile>

    suspend fun blockUser(userId: String)
    suspend fun unblockUser(userId: String)
    suspend fun listBlocked(): List<BlockedUser>

    suspend fun searchByUsername(prefix: String): List<PublicProfile>
    suspend fun searchByPhone(phoneNumber: String): PublicProfile?
    suspend fun getPublicProfile(userId: String): PublicProfile
    suspend fun reportUser(userId: String, reason: String, contextMessageId: String? = null)
}
