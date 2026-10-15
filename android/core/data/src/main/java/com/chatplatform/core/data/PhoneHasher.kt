package com.chatplatform.core.data

import java.security.MessageDigest

/**
 * Computes the same hash the backend expects for /contacts/sync — plain
 * lowercase-hex SHA-256 of the E.164 phone number string, no salt. Must
 * stay byte-for-byte consistent with backend/internal/auth/username.go's
 * hashPhoneNumber(); this is what lets contact sync work without ever
 * uploading a device contact's raw number for numbers that aren't already
 * registered.
 */
object PhoneHasher {
    fun sha256Hex(e164PhoneNumber: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(e164PhoneNumber.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }
}
