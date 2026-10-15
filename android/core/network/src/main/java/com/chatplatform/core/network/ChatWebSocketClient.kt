package com.chatplatform.core.network

import com.google.gson.Gson
import com.google.gson.JsonElement
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import kotlin.math.min
import kotlin.math.pow

data class WsEnvelope(
    val event_id: String,
    val type: String,
    val ts: String,
    val device_id: String? = null,
    val conversation_id: String? = null,
    val version: Int = 1,
    val payload: JsonElement? = null,
)

enum class WsConnectionState { CONNECTING, CONNECTED, DISCONNECTED }

/**
 * Owns exactly one logical WebSocket connection to the chat gateway.
 * Reconnects automatically with exponential backoff + jitter on any
 * unexpected close, matching the client behavior the backend's WS protocol
 * doc assumes (docs/00-architecture-and-roadmap.md section 6).
 */
class ChatWebSocketClient(
    private val client: OkHttpClient,
    private val baseWsUrl: String, // e.g. "ws://10.0.2.2:8080/ws"
    private val accessTokenProvider: suspend () -> String?,
    private val scope: CoroutineScope,
) {
    private val gson = Gson()
    private var socket: WebSocket? = null
    private var reconnectJob: Job? = null
    private var attempt = 0
    private var manuallyClosed = false

    private val _incoming = MutableSharedFlow<WsEnvelope>(extraBufferCapacity = 256)
    val incoming: SharedFlow<WsEnvelope> = _incoming

    private val _state = MutableSharedFlow<WsConnectionState>(replay = 1, extraBufferCapacity = 4)
    val state: SharedFlow<WsConnectionState> = _state

    fun connect() {
        manuallyClosed = false
        scope.launch { doConnect() }
    }

    fun disconnect() {
        manuallyClosed = true
        reconnectJob?.cancel()
        socket?.close(1000, "client closing")
        socket = null
    }

    /** Send an envelope; returns false if there's no live connection right
     *  now (caller should queue it locally — see ChatRepositoryImpl). */
    fun send(envelope: WsEnvelope): Boolean {
        val s = socket ?: return false
        return s.send(gson.toJson(envelope))
    }

    private suspend fun doConnect() {
        val token = accessTokenProvider() ?: return // not logged in; nothing to connect with
        _state.tryEmit(WsConnectionState.CONNECTING)

        val request = Request.Builder()
            .url(baseWsUrl)
            .addHeader("Authorization", "Bearer $token")
            .build()

        socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                attempt = 0
                _state.tryEmit(WsConnectionState.CONNECTED)
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                runCatching { gson.fromJson(text, WsEnvelope::class.java) }
                    .onSuccess { _incoming.tryEmit(it) }
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                _state.tryEmit(WsConnectionState.DISCONNECTED)
                scheduleReconnect()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                _state.tryEmit(WsConnectionState.DISCONNECTED)
                scheduleReconnect()
            }
        })
    }

    private fun scheduleReconnect() {
        if (manuallyClosed) return
        reconnectJob?.cancel()
        attempt += 1
        // base 1s, cap 30s, +/- up to 20% jitter — matches the backend's
        // documented expectation for reconnect behavior.
        val base = min(30_000L, (1000L * 2.0.pow(min(attempt, 6)).toLong()))
        val jitter = (base * 0.2 * (Math.random() * 2 - 1)).toLong()
        val delayMs = (base + jitter).coerceAtLeast(500L)

        reconnectJob = scope.launch {
            delay(delayMs)
            doConnect()
        }
    }
}
