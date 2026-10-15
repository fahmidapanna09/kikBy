package com.chatplatform.core.network

import com.google.gson.Gson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.flowOn
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.BufferedReader

sealed class AiStreamEvent {
    data class Delta(val text: String) : AiStreamEvent()
    data class Done(val message: AiMessageDto) : AiStreamEvent()
    data class StreamError(val message: String) : AiStreamEvent()
}

private data class SendAiMessageBody(val content: String)
private data class DonePayload(val message: AiMessageDto)
private data class DeltaPayload(val text: String)
private data class ErrorPayload(val error: String)

/**
 * Reads the `POST /v1/ai/conversations/{id}/messages` Server-Sent Events
 * response one line at a time. Not Retrofit, on purpose — Retrofit's
 * suspend functions return once, SSE needs to keep handing chunks to the
 * caller as they arrive. Uses the same authenticated OkHttpClient as
 * everything else (so the access-token-refresh authenticator still applies
 * if one is wired at the DI layer) — NOT the unauthenticated client
 * [MediaUploader] uses for presigned S3 URLs.
 */
class AiStreamClient(
    private val client: OkHttpClient,
    private val baseHttpUrl: String, // e.g. "http://10.0.2.2:8080/" — trailing slash
    private val accessTokenProvider: suspend () -> String?,
) {
    private val gson = Gson()

    fun sendMessage(conversationId: String, content: String): Flow<AiStreamEvent> = callbackFlow {
        val token = accessTokenProvider()
        if (token == null) {
            trySend(AiStreamEvent.StreamError("not signed in"))
            close()
            return@callbackFlow
        }

        val url = baseHttpUrl.trimEnd('/') + "/v1/ai/conversations/$conversationId/messages"
        val body = gson.toJson(SendAiMessageBody(content)).toRequestBody("application/json".toMediaType())
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $token")
            .addHeader("Accept", "text/event-stream")
            .post(body)
            .build()

        val call = client.newCall(request)
        call.enqueue(object : okhttp3.Callback {
            override fun onFailure(call: okhttp3.Call, e: java.io.IOException) {
                trySend(AiStreamEvent.StreamError(e.message ?: "network error"))
                close()
            }

            override fun onResponse(call: okhttp3.Call, response: okhttp3.Response) {
                response.use { resp ->
                    if (!resp.isSuccessful) {
                        val errBody = resp.body?.string().orEmpty()
                        trySend(AiStreamEvent.StreamError("http ${resp.code}: $errBody"))
                        close()
                        return
                    }
                    val source = resp.body?.source()
                    if (source == null) {
                        trySend(AiStreamEvent.StreamError("empty response body"))
                        close()
                        return
                    }

                    var currentEvent = "message"
                    val dataLines = StringBuilder()

                    fun flushEvent() {
                        if (dataLines.isEmpty()) return
                        val data = dataLines.toString()
                        dataLines.setLength(0)
                        when (currentEvent) {
                            "delta" -> runCatching { gson.fromJson(data, DeltaPayload::class.java) }
                                .onSuccess { trySend(AiStreamEvent.Delta(it.text)) }
                            "done" -> runCatching { gson.fromJson(data, DonePayload::class.java) }
                                .onSuccess { trySend(AiStreamEvent.Done(it.message)) }
                            "error" -> runCatching { gson.fromJson(data, ErrorPayload::class.java) }
                                .onSuccess { trySend(AiStreamEvent.StreamError(it.error)) }
                        }
                        currentEvent = "message"
                    }

                    val reader: BufferedReader = source.inputStream().bufferedReader()
                    reader.useLines { lines ->
                        for (line in lines) {
                            when {
                                line.startsWith("event:") -> currentEvent = line.removePrefix("event:").trim()
                                line.startsWith("data:") -> dataLines.append(line.removePrefix("data:").trim())
                                line.isBlank() -> flushEvent()
                                // anything else (e.g. SSE comments starting with ':') is ignored
                            }
                        }
                    }
                    flushEvent() // in case the stream ended without a trailing blank line
                    close()
                }
            }
        })

        awaitClose { call.cancel() }
    }.flowOn(Dispatchers.IO)
}
