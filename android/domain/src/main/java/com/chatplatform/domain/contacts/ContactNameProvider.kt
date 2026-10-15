package com.chatplatform.domain.contacts

/**
 * Turns a user id into something a person can recognize. Lives in `domain`
 * so feature modules (chat, groups) can show real names without depending
 * on each other or on the contacts feature — the app's DI layer supplies
 * the implementation.
 */
interface ContactNameProvider {
    /** Best available label, in order of preference: the viewer's own
     *  nickname for that contact, their display name, "@username", and
     *  finally a short id so the UI never shows a blank row. Never throws —
     *  a failed lookup degrades to the short-id fallback. */
    suspend fun labelFor(userId: String): String

    /** Re-reads the contact list (e.g. after adding/renaming a contact). */
    suspend fun refresh()
}
