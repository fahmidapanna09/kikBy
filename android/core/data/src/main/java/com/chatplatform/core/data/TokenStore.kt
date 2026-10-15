package com.chatplatform.core.data

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

/**
 * NOTE: plain DataStore Preferences is NOT encrypted at rest. For a
 * production build, back this with androidx.security's EncryptedFile /
 * Jetpack Security's DataStore + Android Keystore-backed encryption instead.
 * Kept simple here since Phase 1's goal is a working end-to-end auth loop.
 */
class TokenStore(private val dataStore: DataStore<Preferences>) {
    private object Keys {
        val ACCESS_TOKEN = stringPreferencesKey("access_token")
        val REFRESH_TOKEN = stringPreferencesKey("refresh_token")
        val USER_ID = stringPreferencesKey("user_id")
    }

    val accessTokenFlow: Flow<String?> = dataStore.data.map { it[Keys.ACCESS_TOKEN] }
    val userIdFlow: Flow<String?> = dataStore.data.map { it[Keys.USER_ID] }

    suspend fun currentRefreshToken(): String? = dataStore.data.first()[Keys.REFRESH_TOKEN]
    suspend fun currentAccessToken(): String? = dataStore.data.first()[Keys.ACCESS_TOKEN]

    suspend fun save(accessToken: String, refreshToken: String, userId: String) {
        dataStore.edit { prefs ->
            prefs[Keys.ACCESS_TOKEN] = accessToken
            prefs[Keys.REFRESH_TOKEN] = refreshToken
            prefs[Keys.USER_ID] = userId
        }
    }

    suspend fun updateAccessToken(accessToken: String, refreshToken: String) {
        dataStore.edit { prefs ->
            prefs[Keys.ACCESS_TOKEN] = accessToken
            prefs[Keys.REFRESH_TOKEN] = refreshToken
        }
    }

    suspend fun clear() {
        dataStore.edit { it.clear() }
    }
}
