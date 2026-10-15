package com.chatplatform.feature.contacts

import android.content.Context
import android.provider.ContactsContract

data class DeviceContact(val displayName: String?, val phoneNumber: String)

/**
 * Reads the device's contact list. Caller must have already been granted
 * android.permission.READ_CONTACTS — this does not request it itself, to
 * keep permission-request UX (rationale dialogs, etc.) in the Composable
 * layer where it belongs.
 *
 * Phone numbers are returned however the device stores them (not
 * necessarily E.164) — normalize before hashing/sending to
 * ContactsRepository.syncByHashes, e.g. with libphonenumber, since the
 * backend only matches exact E.164-hash equality. Left as a TODO: this
 * function does not attempt normalization itself.
 */
object DeviceContactsReader {
    fun readAll(context: Context): List<DeviceContact> {
        val results = mutableListOf<DeviceContact>()
        val cursor = context.contentResolver.query(
            ContactsContract.CommonDataKinds.Phone.CONTENT_URI,
            arrayOf(ContactsContract.CommonDataKinds.Phone.DISPLAY_NAME, ContactsContract.CommonDataKinds.Phone.NUMBER),
            null, null, null,
        )
        cursor?.use {
            val nameIdx = it.getColumnIndex(ContactsContract.CommonDataKinds.Phone.DISPLAY_NAME)
            val numberIdx = it.getColumnIndex(ContactsContract.CommonDataKinds.Phone.NUMBER)
            while (it.moveToNext()) {
                val name = if (nameIdx >= 0) it.getString(nameIdx) else null
                val number = if (numberIdx >= 0) it.getString(numberIdx) else null
                if (!number.isNullOrBlank()) {
                    results.add(DeviceContact(displayName = name, phoneNumber = number))
                }
            }
        }
        return results
    }
}
