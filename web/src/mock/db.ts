import type { AppNotification, Conversation, Friend, ID, Message, Settings, User } from "@/types/models";
import { PEOPLE } from "./users";
import { uid } from "@/lib/utils";

/** In-memory mock database. State resets on page reload except accounts & settings (localStorage). */
const ago = (min: number) => new Date(Date.now() - min * 60000).toISOString();

export interface Account {
  user: User;
  pwHash: string;
}

/** Mock-only password digest (NOT secure — the real backend uses Argon2id). */
export function mockHash(pw: string): string {
  let h = 2166136261;
  for (let i = 0; i < pw.length; i++) h = Math.imul(h ^ pw.charCodeAt(i), 16777619) >>> 0;
  return `m${h.toString(16)}`;
}

const ACC_KEY = "kikbite.mock.accounts";
const SET_KEY = "kikbite.mock.settings";

export const defaultSettings: Settings = {
  privacy: { lastSeen: "everyone", readReceipts: true, typingIndicator: true, profilePhoto: "everyone" },
  notifications: { messages: true, groups: true, friendRequests: true, sound: true, preview: true },
  appearance: { density: "comfortable", fontScale: 100, reduceMotion: false },
  chat: { enterToSend: true, mediaAutoDownload: true, spellcheck: true },
};

export function loadAccounts(): Account[] {
  try {
    const raw = localStorage.getItem(ACC_KEY);
    if (raw) return JSON.parse(raw) as Account[];
  } catch {
    /* ignore */
  }
  const demo: Account = {
    user: { id: "u_demo", username: "demo", displayName: "Demo User", email: "demo@kikbite.com", bio: "Trying out KikBite.", presence: "online", createdAt: ago(20000) },
    pwHash: mockHash("Demo1234"),
  };
  saveAccounts([demo]);
  return [demo];
}
export function saveAccounts(a: Account[]) {
  try {
    localStorage.setItem(ACC_KEY, JSON.stringify(a));
  } catch {
    /* ignore */
  }
}
export function loadSettings(userId: ID): Settings {
  try {
    const raw = localStorage.getItem(`${SET_KEY}.${userId}`);
    if (raw) return { ...defaultSettings, ...JSON.parse(raw) };
  } catch {
    /* ignore */
  }
  return structuredClone(defaultSettings);
}
export function saveSettings(userId: ID, s: Settings) {
  try {
    localStorage.setItem(`${SET_KEY}.${userId}`, JSON.stringify(s));
  } catch {
    /* ignore */
  }
}

export interface MockState {
  meId: ID;
  users: Record<ID, User>;
  conversations: Conversation[];
  messages: Record<ID, Message[]>; // by conversation, ascending
  friends: Friend[];
  notifications: AppNotification[];
}

let state: MockState | null = null;

const mk = (conversationId: ID, senderId: ID, text: string, minAgo: number, extra: Partial<Message> = {}): Message => ({
  id: uid("m"),
  conversationId,
  senderId,
  text,
  attachments: [],
  reactions: [],
  status: "read",
  createdAt: ago(minAgo),
  ...extra,
});

export function seedFor(me: User): MockState {
  if (state && state.meId === me.id) return state;
  const users: Record<ID, User> = Object.fromEntries([me, ...PEOPLE].map((u) => [u.id, u]));
  const meId = me.id;

  const convs: Conversation[] = [];
  const messages: Record<ID, Message[]> = {};
  const direct = (peerId: ID, lines: Array<[boolean, string, number]>, opts: Partial<Conversation> = {}) => {
    const id = `c_${peerId}`;
    const msgs = lines.map(([mine, text, m]) => mk(id, mine ? meId : peerId, text, m));
    messages[id] = msgs;
    const last = msgs[msgs.length - 1];
    convs.push({
      id, kind: "direct", title: users[peerId].displayName, memberIds: [meId, peerId], peerId,
      lastMessage: { id: last.id, text: last.text, senderId: last.senderId, createdAt: last.createdAt, status: last.status },
      unreadCount: 0, pinned: false, muted: false, archived: false, updatedAt: last.createdAt, ...opts,
    });
  };

  // Long history for Aisha to exercise date separators, replies, reactions, pagination
  const aishaId = "c_u_aisha";
  const aisha: Message[] = [];
  const A = (mine: boolean, text: string, m: number, extra: Partial<Message> = {}) => aisha.push(mk(aishaId, mine ? meId : "u_aisha", text, m, extra));
  for (let d = 5; d >= 3; d--) {
    A(false, `Day -${d}: morning! Did you see the new onboarding flow?`, d * 1440 + 300);
    A(true, "Yes — the phone number step is much cleaner now.", d * 1440 + 290);
    A(false, "Great. I'll polish the empty states next.", d * 1440 + 280);
  }
  A(true, "Quick question — are we shipping the friends tab this week?", 1440 + 200);
  A(false, "Yes! Friend requests + block list are done on my side.", 1440 + 190, { reactions: [{ emoji: "👍", userIds: [meId] }] });
  A(true, "Perfect. I'll review the Figma specs tonight.", 1440 + 180);
  A(false, "Hey! Are we still on for the design review today?", 95);
  const q = aisha[aisha.length - 1];
  A(true, "Yes, 4 PM works for me.", 90);
  A(false, "Can you send the latest mockups before then?", 88, { reactions: [{ emoji: "🔥", userIds: [meId, "u_aisha"] }] });
  A(true, "Sure — sending them over now.", 85, { replyToId: q.id });
  A(false, "Thanks! Looks great, the dark theme feels really premium.", 4);
  A(false, "One more thing: can we make the send button a bit bigger on mobile?", 3);
  messages[aishaId] = aisha;
  const lastA = aisha[aisha.length - 1];
  convs.push({
    id: aishaId, kind: "direct", title: users.u_aisha.displayName, memberIds: [meId, "u_aisha"], peerId: "u_aisha",
    lastMessage: { id: lastA.id, text: lastA.text, senderId: "u_aisha", createdAt: lastA.createdAt, status: "delivered" },
    unreadCount: 2, pinned: true, muted: false, archived: false, updatedAt: lastA.createdAt,
  });

  direct("u_rafi", [[false, "API contract is merged 🎉", 140], [true, "Nice! Pulling it now.", 130], [false, "Ping me if the types don't line up.", 128]], { unreadCount: 1, updatedAt: ago(128) });
  direct("u_nabila", [[true, "Lunch tomorrow?", 600], [false, "Sure, 1 PM at the usual place.", 590]], { pinned: true });
  direct("u_tanvir", [[false, "Sent you the invoice PDF.", 3000], [true, "Got it, thanks!", 2990]]);
  direct("u_mim", [[false, "That bug is fixed, can you re-test?", 25], [true, "On it.", 22]], { unreadCount: 3 });
  direct("u_sakib", [[true, "See you at the meetup 👋", 6000]], { muted: true });

  // Group chat
  const gid = "c_team";
  const gm: Message[] = [
    mk(gid, "u_rafi", "Standup in 5 minutes", 200),
    mk(gid, "u_mim", "Joining!", 198),
    mk(gid, meId, "Same, give me 2 mins.", 196),
    mk(gid, "u_aisha", "Sprint demo is Friday 3 PM.", 60),
    mk(gid, "u_rafi", "I'll prepare the staging build.", 55),
  ];
  messages[gid] = gm;
  convs.push({
    id: gid, kind: "group", title: "KikBite Team", memberIds: [meId, "u_aisha", "u_rafi", "u_mim", "u_nabila"],
    lastMessage: { id: gm[4].id, text: gm[4].text, senderId: "u_rafi", createdAt: gm[4].createdAt, status: "read" },
    unreadCount: 5, pinned: false, muted: false, archived: false, updatedAt: gm[4].createdAt,
  });

  const friends: Friend[] = [
    ...["u_aisha", "u_rafi", "u_nabila", "u_tanvir", "u_mim", "u_sakib"].map((id, i): Friend => ({ user: users[id], status: "friends", since: ago(20000 + i * 900), mutualCount: 3 + i })),
    { user: users.u_farhan, status: "incoming", mutualCount: 2 },
    { user: users.u_lamia, status: "incoming", mutualCount: 5 },
    { user: users.u_sadia, status: "outgoing", mutualCount: 1 },
    { user: users.u_imran, status: "blocked" },
  ];

  const notifications: AppNotification[] = [
    { id: uid("n"), kind: "friend_request", title: "Farhan Ali", body: "sent you a friend request", actorId: "u_farhan", read: false, createdAt: ago(18) },
    { id: uid("n"), kind: "message", title: "Aisha Rahman", body: "One more thing: can we make the send button…", actorId: "u_aisha", conversationId: aishaId, read: false, createdAt: ago(3) },
    { id: uid("n"), kind: "mention", title: "KikBite Team", body: "Rafi mentioned you: “@you can you review the PR?”", actorId: "u_rafi", conversationId: gid, read: false, createdAt: ago(70) },
    { id: uid("n"), kind: "friend_accept", title: "Mim Chowdhury", body: "accepted your friend request", actorId: "u_mim", read: true, createdAt: ago(2900) },
    { id: uid("n"), kind: "system", title: "Welcome to KikBite", body: "Small bites. Big conversations. Complete your profile to get started.", read: true, createdAt: ago(20000) },
  ];

  state = { meId, users, conversations: convs, messages, friends, notifications };
  return state;
}

export function db(): MockState {
  if (!state) throw new Error("mock db not seeded");
  return state;
}
export function resetDb() {
  state = null;
}
