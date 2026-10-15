# Android — Phases 1–7 (Auth + Chat + Media/Voice + Groups/Reactions/Search + Contacts + Stories + Communities)

- **Phase 1:** phone entry → OTP verification, wired to the backend auth endpoints.
- **Phase 2:** conversation list + chat screen backed by a local Room cache
  (works offline), a WebSocket client with auto-reconnect, an offline send
  queue with retry, and delivery/read receipts.
- **Phase 3:** image/document picking with upload progress, voice-message
  recording, and image/voice bubbles in the chat screen.
- **Phase 4:** message reactions (long-press a bubble), forwarding (to any
  existing conversation), and a search screen — see "What's deferred in
  Phase 4" below for what's intentionally *not* here yet.
- **Phase 5:** contacts (add manually, sync from the device address book,
  block/unblock), username search, and — built on top of them — the group
  member picker and real names in the forward dialog (see "Real names and
  group screens" below).
- **Phase 6:** 24-hour Stories/Status — a tray, a full-screen auto-advancing
  viewer with reply/react, posting (text or photo/video), your own archive
  and viewer list, and muting someone's stories. See "Stories (Phase 6)"
  below for what's deferred.
- **Phase 7:** Communities — a "Communities" tab (yours + search + join by
  code), creating one (with its announcement group made automatically),
  linking/creating groups inside it, member roles, invite links, ban, and
  reporting. See "Communities (Phase 7)" below for the one real cross-module
  wiring point it needs.

## What's NOT here, and why

This is source files only, not a buildable Gradle project — no
`settings.gradle.kts`, per-module `build.gradle.kts`, Gradle wrapper, Hilt DI
modules, `AndroidManifest.xml`, or app-level `MainActivity`/`NavHost`.
Hand-typing those without being able to run `./gradlew build` here would mean
shipping boilerplate I can't verify compiles — generate that scaffold with
Android Studio's "New Project" (Compose, Kotlin, Hilt, multi-module
template) or `gradle init`, then drop these files into the matching module
folders below.

## File layout

```
core/network/.../{ApiConfig,AuthApi,AuthInterceptors,ChatApi,ChatWebSocketClient,MediaApi,MediaUploader,GroupApi,ContactsApi}.kt
core/data/.../{TokenStore,PhoneHasher}.kt
core/data/.../db/{MessageEntity,ConversationEntity,MessageDao,ConversationDao,AppDatabase}.kt
domain/.../auth/AuthRepository.kt
domain/.../chat/ChatRepository.kt
domain/.../media/MediaRepository.kt
domain/.../contacts/{ContactsRepository,ContactNameProvider}.kt
domain/.../groups/GroupsRepository.kt
domain/.../stories/StoriesRepository.kt
domain/.../communities/CommunitiesRepository.kt
data/.../auth/AuthRepositoryImpl.kt
data/.../chat/ChatRepositoryImpl.kt
data/.../media/MediaRepositoryImpl.kt
data/.../contacts/{ContactsRepositoryImpl,ContactNameProviderImpl}.kt
data/.../groups/GroupsRepositoryImpl.kt
data/.../stories/StoriesRepositoryImpl.kt
data/.../communities/CommunitiesRepositoryImpl.kt
feature/auth/.../{AuthViewModel,PhoneEntryScreen,OtpEntryScreen,AuthRoute}.kt
feature/chat/.../{ChatViewModel,ChatScreen,VoiceRecorder,SearchViewModel,SearchScreen}.kt
feature/contacts/.../{ContactsViewModel,ContactsScreen,DeviceContactsReader}.kt
feature/groups/.../{MemberPickerViewModel,MemberPicker,CreateGroupViewModel,CreateGroupScreen,GroupInfoViewModel,GroupInfoScreen,AddMembersScreen}.kt
feature/stories/.../{StoriesTrayViewModel,StoriesTray,StoryViewerViewModel,StoryViewerScreen,CreateStoryViewModel,CreateStoryScreen,StoryViewersScreen}.kt
feature/communities/.../{CommunitiesListViewModel,CommunitiesListScreen,CreateCommunityViewModel,CreateCommunityScreen,CommunityDetailViewModel,CommunityDetailScreen}.kt
```

Module dependency direction: `domain` is pure Kotlin (no Android
dependency); `core/network` and `core/data` depend on
Retrofit/OkHttp/DataStore/Room; `data` depends on `domain` + `core/*`;
`feature/*` depends on `domain` + AndroidX Compose/ViewModel. `data` is the
only layer allowed to know about `core/network` DTOs — `domain` and
`feature` only ever see the domain models (`ChatMessage`, `Conversation`,
etc.). `feature/contacts` does not depend on `feature/chat` or vice versa —
see "Real names and group screens" below for how the two are connected.

## Required manifest addition (Phase 5)
```xml
<uses-permission android:name="android.permission.READ_CONTACTS" />
```

## Stories (Phase 6)

**Wiring note — shares the chat WebSocket, doesn't open a second one:**
`StoriesRepositoryImpl` takes the *same* `ChatWebSocketClient` instance
`ChatRepositoryImpl` uses and just also listens for `"story.created"`
frames on it. Wire one `ChatWebSocketClient` singleton at your DI layer and
hand it to both repositories — creating a second client per repository would
mean two live connections to the same account, doubling server-side
presence/connection bookkeeping for no benefit.

```kotlin
val storiesRepository = StoriesRepositoryImpl(storiesApi, sharedChatWebSocketClient)

StoriesTray(
    StoriesTrayViewModel(storiesRepository),
    onOpenMine = { /* open StoryViewerScreen with isOwnStories = true */ },
    onAddStory = { /* open CreateStoryScreen */ },
    onOpenEntry = { entry -> /* open StoryViewerScreen at that entry's index */ },
)
```

`StoryViewerViewModel` drives the full-screen viewer itself: auto-advances
through one author's stories then the next (5s for text, a fixed 7s for
media — see the "Known gaps" note on video below), marks each story viewed
as it's shown, and exposes reply/react/delete. Tap the left third of the
screen for the previous story, anywhere else for next, press-and-hold to
pause — the same interaction WhatsApp/Instagram stories use, implemented
with a single `pointerInput(onPress = ...)` rather than a library.

### Known gaps in Phase 6
- **No video playback.** `StoryViewerScreen` shows a placeholder for
  `StoryType.VIDEO` — wiring in Media3/ExoPlayer (and using the video's
  real duration instead of the fixed 7s fallback) is a contained follow-up,
  not done here to avoid adding a new dependency in this pass.
- **Only `privacy: contacts` is postable from the UI.** The backend also
  supports `contacts_except` and `selected`, but building their audience
  picker would duplicate `feature/groups`'s `MemberPicker` — see that
  module's own notes on why cross-feature UI dependencies are avoided here.
  Promoting `MemberPicker` to a shared location (`core` or its own module)
  is the natural fix if/when this is picked up.
- **No mention picker** when composing a story (the backend's
  `mention_user_ids` field is unused from the UI for the same reason as above).
- `StoryViewersScreen` fetches once on open rather than through a
  refreshable ViewModel — fine for a short-lived detail screen, but it
  won't update live if more views come in while it's open.
- GIFs work as an `image` story server-side (`image/gif` is an allowed MIME
  type), but `CreateStoryScreen`'s picker doesn't distinguish them from
  regular photos — no GIF-specific picker or preview.

## Communities (Phase 7)

A community's announcement group is a real group underneath (same `groups`
table, same `conversation_id` the chat module uses) — `CommunityDetailScreen`
opens it with `onOpenGroup`, which you wire to the exact same chat screen
every other group uses. There's no separate "community chat" UI; the
community is a container around ordinary groups plus membership/roles.

**The one real wiring gap:** adding an *existing* group to a community needs
a list of groups you admin, which lives in the chat module's conversation
list — and `feature/communities` deliberately doesn't depend on
`feature/chat` (same reasoning as `feature/chat` not depending on
`feature/contacts` earlier). `CommunityDetailViewModel` takes a
`myGroupsProvider: suspend () -> List<GroupOption>` lambda for exactly this,
supplied at your DI/nav layer:

```kotlin
val myGroupsProvider: suspend () -> List<GroupOption> = {
    chatRepository.observeConversations().first()
        .filter { it.type == "group" }
        .map { GroupOption(it.id, it.name ?: "Group") }
    // A real implementation also filters to groups you admin/own — the
    // domain Conversation model doesn't carry your role in it today, so
    // this version offers every group you're in; the backend's own
    // AddExistingGroup check (admin/owner of that group) still enforces
    // the real rule, this is just what's offered in the UI.
}

CommunityDetailViewModel(communitiesRepository, names, communityId, myUserId, myGroupsProvider)
```

### Known gaps in Phase 7
- The `myGroupsProvider` default (no groups offered) means "Add group" only
  really works once you wire the lambda above — creating a *new* group from
  the dialog works out of the box either way.
- No community icon upload UI (the backend field exists; `CreateCommunityScreen`
  doesn't expose it, consistent with groups not having one yet either).
- `CommunitiesListScreen`'s search and "yours" lists don't distinguish a
  community you were banned from (you'd see it in search results, then get
  a `banned` error on tapping Join) — no pre-emptive "you can't rejoin" label.
- Same as groups: an owner can't leave (hidden button) because there's no
  ownership transfer on the backend.

## Channels (Phase 8)

One-way broadcast, so the Android side is deliberately asymmetric: every
follower gets `ChannelDetailScreen`'s read-only feed (post cards, reactions,
poll voting), and only an admin/owner additionally sees the composer row
(text/photo/video/poll) and the admin menu (add/remove admin, invite link,
delete channel). `ChannelDetailViewModel` checks `channel.isAdmin` from the
backend response to decide which UI to show — it never trusts a
client-side flag the user could have stale.

Like Stories and Chat, `ChannelsRepositoryImpl` shares the *same*
`ChatWebSocketClient` instance (wire one client, not three) and just
filters `"channel.post"` out of the shared `incoming: Flow<WsEnvelope>` —
`channelPostEvents: Flow<Pair<String, String>>` (channel id, post id) is
what `ChannelDetailViewModel` collects to live-refresh the feed when a new
post lands while the screen is open.

```kotlin
// at your DI/nav layer, alongside the Stories/Chat wiring:
val channelsRepository: ChannelsRepository = ChannelsRepositoryImpl(channelsApi, chatWebSocketClient, mediaRepository)
```

### Known gaps in Phase 8 (Android)
- No markdown/rich-text rendering for post bodies or link previews — plain
  text and a raw URL, matching the backend not fetching link previews
  either (see `backend/README.md`'s Phase 8 known gaps).
- Poll voting UI shows percentage bars once you've voted; there's no
  "live" re-tally while the screen sits open unless a new post event
  happens to trigger a refresh — pull-to-refresh isn't wired either.
- Sticker posts render as a plain image — no distinct sticker tray/picker
  in `CreateChannelScreen`'s or the composer's media attach flow.
- Invite-code screen is text-only (shows and lets you copy the code) — no
  QR code generation/scanning.

## AI Agent (Phase 9)

A single `AiChatScreen` per conversation, streamed. `AiStreamClient`
(in `core/network`, next to `MediaUploader` and `ChatWebSocketClient`) makes
a raw OkHttp `POST` to `.../ai/conversations/{id}/messages` with
`Accept: text/event-stream` and reads the response body line-by-line as
Server-Sent Events — Retrofit's suspend functions return once, which
doesn't fit "keep handing me chunks," so this one deliberately isn't a
Retrofit call like the rest of `AiApi`. It's exposed as a cold
`Flow<AiStreamEvent>`: collecting it is what sends the request, and
cancelling collection (e.g. leaving the screen) cancels the underlying
OkHttp call.

`AiChatViewModel.send()` adds an optimistic local bubble for what you typed
(so it's on screen immediately, before the first delta arrives), accumulates
deltas into a temporary "streaming" bubble at the bottom of the list, and on
`AiTurnEvent.Completed` reloads the real history from the server — which
replaces both the optimistic bubble and the streaming one with the
canonical persisted pair. A failed turn (`AiTurnEvent.Failed`, e.g. the
429 quota response) surfaces as a Snackbar and leaves your optimistic bubble
in place so you don't lose what you typed.

```kotlin
// at your DI/nav layer:
val aiStreamClient = AiStreamClient(authenticatedOkHttpClient, baseHttpUrl, accessTokenProvider)
val aiRepository: AiRepository = AiRepositoryImpl(aiApi, aiStreamClient)

// list screen:
AiConversationListScreen(AiConversationListViewModel(aiRepository), onOpenConversation = { id -> navigate(id) })
// chat screen, one ViewModel per open conversation id:
AiChatScreen(AiChatViewModel(aiRepository, conversationId))
```

Same isolation rule as the backend: `feature/ai`, `domain/ai`, and
`data/ai` have no dependency on `feature/chat`, `domain/contacts`, or
anything else — the agent only ever sees what's typed directly into it.

### Known gaps in Phase 9 (Android)
- **No markdown rendering** for the model's replies — plain `Text`, so
  lists/code blocks/bold come back as raw characters if Gemini sends them.
- **No "stop generating" button** — leaving the screen cancels the stream
  (see above), but there's no in-screen control to cancel mid-reply and
  keep the screen open.
- **No local/offline cache** — unlike chat messages (Room-backed), AI
  conversations and messages are fetched live every time; no offline
  history, no local search.
- **No image/voice input** into the AI composer — text only, matching the
  backend's text-in/text-out-only scope for this phase.
- **No retry button** on a failed turn — you have to retype and resend.

## Security hardening (Phase 10)

The Android-side change in this phase is small and additive:
`AuthRepository.revokeAllSessions()` (backed by
`POST /v1/auth/sessions/revoke-all`) is the "log out everywhere" / "I
think my account is compromised" action. `AuthRepositoryImpl` clears local
tokens immediately on a successful call, the same way `logout()` already
does, since this device's own session is among the ones just revoked
server-side.

```kotlin
// wherever a "log out of all devices" action belongs once a settings/
// sessions screen exists — one line against the existing repository:
authViewModel.revokeAllSessions() // -> authRepository.revokeAllSessions()
```

**No new screen was built for this** — `listSessions()`/`revokeSession()`
already existed in `AuthRepository` with no dedicated UI screen consuming
them (this scaffold's `feature/auth` only ever had phone-entry and
OTP-entry screens), so `revokeAllSessions()` matches that same
already-established level of completeness rather than introducing a new
Settings feature module as a side effect of a security-hardening phase.

The bulk of Phase 10 is backend-only (OTP hashing, security headers,
audit logging, the E2EE threat model design doc) — see
`backend/README.md`'s own Phase 10 section and
`backend/docs/E2EE_THREAT_MODEL.md`. **End-to-end encryption is a design
document only in this phase, not implemented anywhere in this app** —
message content sent from Android today is exactly as protected (TLS in
transit, plaintext at the backend) as it was in every prior phase.

## Admin panel + observability (Phase 11)

**No Android changes this phase, by design.** Phase 11 added an
`/v1/admin` API (RBAC roles, user suspension, report resolution, growth
metrics) and a `/metrics` Prometheus endpoint — both are ops/moderator
tooling, not an end-user-facing feature. The architecture doc's Android
screen list never included an admin surface, so there is no
`feature/admin` module, no `AdminApi.kt`, and nothing to wire up here —
adding one now would be scope the roadmap never asked for. If an
internal moderation app is ever wanted, it's a new, separate client
against the same `/v1/admin` endpoints documented in `backend/README.md`,
not a module bolted onto this consumer-facing app.

## Load testing + AWS deployment (Phase 12)

**No Android changes this phase either.** Phase 12 is backend
infrastructure and load testing only — AWS Terraform, pgbench/k6 load
tests, a deployment runbook (see `backend/README.md`'s own Phase 12
section and `backend/terraform/`/`backend/loadtest/`). The one thing
that does matter here: once a real deployment exists, point `ApiConfig`'s
`BASE_URL`/WS URL at it instead of `10.0.2.2:8080` to test this app
against something other than a laptop — `backend/README.md`'s "Deploying
somewhere reachable for QA/testing" section has the exact host to use.
Nothing about the app's own reconnect/backoff logic changed; Phase 12's
WS-failover drill (`backend/terraform/RUNBOOK.md` step 7) exists
specifically to verify that *existing* logic holds up against a real
multi-node backend, not to add new client-side behavior.

## Real names and group screens (now wired)

**Names.** `ContactNameProvider` (domain interface, `ContactNameProviderImpl`
in `data`) turns a user id into the best label available: your nickname for
them, then their display name, then `@username`, then a short id as a last
resort. `ChatViewModel` takes one in its constructor and uses it for the
forward dialog, which now lists real contact names and group names instead of
raw ids. Groups get their name from the conversation list itself (the
backend now returns `name` for group conversations). Because the resolver
lives in `domain`, `feature/chat` and `feature/groups` still don't depend on
`feature/contacts` or on each other.

**Group screens** (`feature/groups`):
- `CreateGroupScreen` — name, optional description, and a `MemberPicker`
  (your registered contacts, plus username search for anyone else).
- `GroupInfoScreen` — member list with real names and Owner/Admin labels;
  owners/admins get a per-member menu (make admin, remove admin, remove from
  group), an invite-code section, and everyone else gets "Leave group".
- `AddMembersScreen` — the same picker, excluding people already in the group.

**Hooking them up** (your nav/DI layer — there isn't one in this scaffold):
```kotlin
val names = ContactNameProviderImpl(contactsRepository)
ChatViewModel(chatRepository, mediaRepository, names, conversationId)

// New group: exclude yourself from the picker
CreateGroupScreen(
    CreateGroupViewModel(groupsRepository, chatRepository),
    MemberPickerViewModel(contactsRepository, excludedUserIds = setOf(myUserId)),
    onCreated = { conversationId -> /* navigate to the chat */ },
)

// Add members: exclude everyone already in the group
val info = GroupInfoViewModel(groupsRepository, names, chatRepository, conversationId, myUserId)
AddMembersScreen(MemberPickerViewModel(contactsRepository, info.currentMemberIds), info, onDone = { /* back */ })
```
Creating a group or changing its membership makes the backend push a
`group.updated` / `group.member_*` event to every member, and
`ChatRepositoryImpl` responds by re-pulling the conversation list — so a
newly-added person sees the group appear, and someone removed sees it vanish
(the refresh now also deletes conversations the server no longer lists).

**Known gaps here:**
- **Owners can't leave** (the button is hidden) because the backend has no
  ownership transfer. The API itself would still let an owner remove
  themselves, stranding the group — a server-side guard or auto-transfer is
  the real fix.
- An invite code is only shown right after you create it; the server never
  returns an existing one when you just open the group.
- Invite codes aren't wrapped in a shareable deep link — there's no URL
  scheme defined yet, so "Copy" copies the bare code.
- `SearchScreen` results still label matches "in conversation ab12cd34…";
  it wasn't switched over to names in this pass.
- Name lookups for people who aren't saved contacts run one at a time (each
  is a `GET /users/{id}`, then cached), so a first open of a large group can
  be slow.
- Room's `ConversationEntity` gained a `name` column — same schema-version
  note as the other Phase 4/5 columns.

## What's deferred in Phase 4, and why

**Group creation/management UI** was deferred in Phase 4 pending a way to
pick people; Phase 5 added contacts and the screens above now use them.

Also deferred:
- **Jump-to-message** from a search result: `SearchScreen` reports which
  conversation a match is in, but doesn't scroll `ChatScreen` to that
  message — there's no navigation graph yet for it to hook into (see "What's
  NOT here" above).
- **Reaction picker is a fixed 6-emoji row**, not a full emoji keyboard.
- Reactions and the outbox aren't linked: `setReaction`/`removeReaction`
  fire-and-forget over REST (see `ChatRepositoryImpl`) rather than going
  through the offline queue — a reaction attempted while offline is
  currently just dropped, unlike message sends.
- Adding the `role`/`forwardedFromMessageId` columns means bumping Room's
  database `version` (currently `1`) and adding a migration, or using
  `fallbackToDestructiveMigration()` for local development — not done here
  since there's no shipped schema yet to migrate *from*.

## Known gaps in Phase 5 (being upfront about scope)
- `DeviceContactsReader` returns phone numbers exactly as stored on the
  device — not normalized to E.164. The backend's sync match is an exact
  hash comparison, so a number saved locally as `01712-345678` won't match
  even if it's the same number as someone's registered `+8801712345678`.
  A real implementation needs a normalization step (e.g. Google's
  libphonenumber) between `DeviceContactsReader` and `PhoneHasher` — not
  included here to avoid pulling in a large dependency for this pass.
- No contact-request/approval flow — matches the backend's WhatsApp-style
  design (adding is unilateral, not something the other person accepts).
- Blocking has no confirmation dialog in `ContactsScreen` — tapping "Block"
  in the row menu fires immediately.
- `getPublicProfile`/`reportUser` exist on `ContactsApi`/`UsersApi` but have
  no UI entry point yet (e.g. from a chat's profile header, which doesn't
  exist in this scaffold).


## How the offline-first chat flow works (Phase 2, extended by Phase 3)

`sendMessage()` writes to Room immediately (the UI updates instantly, marked
"Sending…") and returns — it never blocks on the network. A background
collector tries the WebSocket first, falls back to REST if the socket is
down, and leaves the message as `PENDING` (not `FAILED`) on a network error
so it's automatically retried from `MessageDao.getOutbox()` the next time the
socket reconnects. `client_tmp_id` is what makes retries safe: the backend's
unique index on `(conversation_id, sender_id, client_tmp_id)` means sending
the same pending row twice is a no-op server-side, not a duplicate message.

Attachments (Phase 3) plug into the same path: `MediaRepository.upload()`
does presign → PUT → confirm and hands back a `mediaId`;
`ChatViewModel.sendAttachment()` then calls
`ChatRepository.sendMessage(..., attachmentMediaIds = listOf(mediaId))`,
which is the exact same offline-queued send as a text message. Download URLs
are always fetched fresh right before use (`downloadUrlFor` inside a
`LaunchedEffect`) rather than cached, since presigned URLs expire.

## Required manifest additions (Phase 3)
```xml
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.RECORD_AUDIO" />
```

## Gradle dependencies needed
- `androidx.room:room-runtime` + `room-ktx` + `room-compiler` (KSP) — Phase 2
- `com.squareup.okhttp3:okhttp` (WebSocket support is built in, no separate lib) — Phase 2
- `com.google.code.gson:gson` — Phase 2
- `androidx.compose.material:material-icons-extended` — Phase 2 (send icon), Phase 3 (attach/mic/play icons)
- `io.coil-kt:coil-compose` — Phase 3, used by `ImageAttachment` in `ChatScreen.kt`
- No extra dependency for voice recording/playback — `MediaRecorder` and `MediaPlayer` are both part of the Android SDK

`ChatRepositoryImpl` uses `java.time.Instant`, which needs either
`minSdk 26+` or Java 8+ API desugaring enabled
(`isCoreLibraryDesugaringEnabled = true` + the `coreLibraryDesugaring`
dependency) if you're targeting older devices.

## Known gaps in Phase 3 (being upfront about scope)
- Document attachments (PDFs etc.) upload and send correctly but render as
  plain `[document]` text in the chat bubble — no file-icon/filename/size UI yet.
- The mic button is a simple press-and-hold; there's no "slide left to
  cancel" gesture (WhatsApp-style) — releasing always sends whatever was
  recorded. A cancel gesture is a natural follow-up, not implemented here.
- Voice playback uses `MediaPlayer.prepare()` (blocking) for simplicity;
  swap for `prepareAsync()` + a listener before shipping, so a slow network
  can't stall the UI thread.

## Pointing the app at your test backend
Change `ApiConfig.BASE_URL` and `ApiConfig.WS_URL` — see
`backend/README.md`'s deployment section for where that backend might
actually be running (local Docker Compose, Fly.io/Railway, or an EC2 box).
