package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

data class AddContactBody(val phone_number: String, val nickname: String? = null)
data class UpdateContactBody(val nickname: String)
data class ContactDto(
    val id: String,
    val contact_user_id: String?,
    val phone_number: String,
    val nickname: String?,
    val is_registered: Boolean,
    val username: String?,
    val display_name: String?,
)
data class ContactsResponseDto(val contacts: List<ContactDto>?)

data class SyncBody(val phone_hashes: List<String>)
data class SyncMatchDto(val phone_hash: String, val user_id: String, val username: String, val display_name: String?)
data class SyncResponseDto(val matches: List<SyncMatchDto>?)

data class BlockedUserDto(val user_id: String, val username: String, val display_name: String?, val blocked_at: String)
data class BlockedResponseDto(val blocked: List<BlockedUserDto>?)

data class PublicProfileDto(val user_id: String, val username: String, val display_name: String?, val about: String?, val avatar_media_id: String?)
data class UserSearchResponseDto(val users: List<PublicProfileDto>?)
data class UpdateUsernameBody(val username: String)
data class MeResponseDto(
    val user_id: String,
    val phone_number: String,
    val username: String,
    val display_name: String?,
    val about: String?,
    val avatar_media_id: String?,
    val last_seen_visibility: String,
)
data class UpdateProfileBody(val display_name: String? = null, val about: String? = null, val avatar_media_id: String? = null, val last_seen_visibility: String? = null)
data class ReportUserBody(val reason: String, val context_message_id: String? = null)

interface ContactsApi {
    @POST("v1/contacts")
    suspend fun addContact(@Body body: AddContactBody): ContactDto

    @GET("v1/contacts")
    suspend fun listContacts(): ContactsResponseDto

    @PATCH("v1/contacts/{contactId}")
    suspend fun updateContact(@Path("contactId") contactId: String, @Body body: UpdateContactBody)

    @DELETE("v1/contacts/{contactId}")
    suspend fun deleteContact(@Path("contactId") contactId: String)

    @POST("v1/contacts/sync")
    suspend fun sync(@Body body: SyncBody): SyncResponseDto

    @POST("v1/contacts/block/{userId}")
    suspend fun blockUser(@Path("userId") userId: String)

    @DELETE("v1/contacts/block/{userId}")
    suspend fun unblockUser(@Path("userId") userId: String)

    @GET("v1/contacts/blocked")
    suspend fun listBlocked(): BlockedResponseDto
}

interface UsersApi {
    @GET("v1/users/me")
    suspend fun getMe(): MeResponseDto

    @PATCH("v1/users/me")
    suspend fun updateMe(@Body body: UpdateProfileBody): MeResponseDto

    @PATCH("v1/users/username")
    suspend fun updateUsername(@Body body: UpdateUsernameBody): MeResponseDto

    @GET("v1/users/{userId}")
    suspend fun getPublicProfile(@Path("userId") userId: String): PublicProfileDto

    @GET("v1/users/search")
    suspend fun search(@Query("username") username: String? = null, @Query("phone") phone: String? = null): UserSearchResponseDto

    @POST("v1/users/{userId}/report")
    suspend fun reportUser(@Path("userId") userId: String, @Body body: ReportUserBody)
}
