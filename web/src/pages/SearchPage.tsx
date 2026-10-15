import { useEffect, useMemo, useRef, useState } from "react";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { EmptyState, ErrorState } from "@/components/common/EmptyState";
import { FriendButtons } from "@/components/common/FriendButtons";
import { ListSkeleton } from "@/components/common/Skeleton";
import { UserRow } from "@/components/common/UserRow";
import { Icon } from "@/icons/Icon";
import { Link, navigate, useLocation } from "@/lib/router";
import { recentSearches, useRecentSearches } from "@/lib/recent";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { cacheUsers } from "@/state/actions";
import { useApp } from "@/state/store";
import type { User } from "@/types/models";

type Tab = "people" | "chats";

function Highlight({ text, q }: { text: string; q: string }) {
  const i = q ? text.toLowerCase().indexOf(q.toLowerCase()) : -1;
  if (i < 0) return <>{text}</>;
  return <>{text.slice(0, i)}<mark className="hl">{text.slice(i, i + q.length)}</mark>{text.slice(i + q.length)}</>;
}

export default function SearchPage() {
  const { query } = useLocation();
  const urlQ = query.get("q") ?? "";
  const [input, setInput] = useState(urlQ);
  const [q, setQ] = useState(urlQ); // debounced
  const [tab, setTab] = useState<Tab>("people");
  const [people, setPeople] = useState<User[]>([]);
  const [state, setState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [err, setErr] = useState("");
  const seq = useRef(0);
  const pushed = useRef(urlQ); // last URL value we wrote ourselves
  const recents = useRecentSearches();
  const convs = useApp((s) => s.conversations);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => inputRef.current?.focus(), []);
  // Debounce typing → q (300ms) and mirror to the URL without piling up history entries.
  useEffect(() => {
    const t = setTimeout(() => {
      setQ(input.trim());
      pushed.current = input.trim();
      navigate(`/search${input.trim() ? `?q=${encodeURIComponent(input.trim())}` : ""}`, { replace: true });
    }, 300);
    return () => clearTimeout(t);
  }, [input]);
  // Follow external URL changes (e.g. top-bar search form).
  useEffect(() => { if (urlQ !== pushed.current) { pushed.current = urlQ; setInput(urlQ); } }, [urlQ]); // eslint-disable-line react-hooks/exhaustive-deps

  const run = () => {
    if (q.replace(/^@/, "").length < 2) { setPeople([]); setState("idle"); return; }
    const my = ++seq.current; // ignore out-of-order responses
    setState("loading");
    services.users.search(q).then((r) => {
      if (my !== seq.current) return;
      cacheUsers(r);
      setPeople(r);
      setState("ready");
      recentSearches.add(q);
    }).catch((e) => { if (my === seq.current) { setErr(errorMessage(e, "Search failed.")); setState("error"); } });
  };
  useEffect(run, [q]); // eslint-disable-line react-hooks/exhaustive-deps

  const chatHits = useMemo(() => {
    const n = q.toLowerCase();
    return n.length < 2 ? [] : convs.filter((c) => c.title.toLowerCase().includes(n) || (c.lastMessage?.text ?? "").toLowerCase().includes(n));
  }, [convs, q]);

  return (
    <div className="page">
      <div className="page-inner">
        <h2 className="page-title">Search</h2>
        <form role="search" onSubmit={(e: any) => e.preventDefault()}>
          <div className="input-wrap">
            <Icon name="search" />
            <input ref={inputRef} className="input" type="search" value={input} onChange={(e: any) => setInput(e.target.value)} placeholder="Search by name or @username" aria-label="Search people and chats" maxLength={60} autoComplete="off" />
            {input && <button type="button" className="icon-btn" style={{ width: 32, height: 32 }} aria-label="Clear search" onClick={() => { setInput(""); inputRef.current?.focus(); }}><Icon name="close" size={16} /></button>}
          </div>
        </form>

        {q.replace(/^@/, "").length < 2 ? (
          recents.length > 0 ? (
            <section className="card" aria-labelledby="rec-h">
              <div className="card-head"><h3 id="rec-h">Recent searches</h3><button type="button" className="link-btn" onClick={() => recentSearches.clear()}>Clear all</button></div>
              <ul>
                {recents.map((r) => (
                  <li key={r} className="recent-row">
                    <button type="button" className="recent-main" onClick={() => setInput(r)}><Icon name="clock" size={16} /><span className="truncate">{r}</span></button>
                    <button type="button" className="icon-btn" style={{ width: 32, height: 32 }} aria-label={`Remove ${r} from recent searches`} onClick={() => recentSearches.remove(r)}><Icon name="close" size={14} /></button>
                  </li>
                ))}
              </ul>
            </section>
          ) : <section className="card" style={{ display: "flex" }}><EmptyState icon="search" title="Find people" message="Type at least 2 characters of a name or @username." /></section>
        ) : (
          <>
            <div className="tabs" role="tablist" aria-label="Result type">
              <button type="button" role="tab" className="tab" aria-selected={tab === "people"} onClick={() => setTab("people")}>People{state === "ready" ? ` (${people.length})` : ""}</button>
              <button type="button" role="tab" className="tab" aria-selected={tab === "chats"} onClick={() => setTab("chats")}>Chats ({chatHits.length})</button>
            </div>
            <section className="card" aria-live="polite">
              {tab === "people" ? (
                state === "loading" ? <ListSkeleton rows={4} /> :
                state === "error" ? <ErrorState message={err} onRetry={run} /> :
                people.length === 0 ? <EmptyState icon="user" title="No people found" message={`No one matches “${q}”. Check the spelling or try their @username.`} /> :
                <ul className="user-list">{people.map((u) => <li key={u.id}><UserRow user={u} actions={<FriendButtons user={u} compact />} /></li>)}</ul>
              ) : chatHits.length === 0 ? (
                <EmptyState icon="chat" title="No chats found" message={`Nothing in your chats matches “${q}”.`} />
              ) : (
                <ul>{chatHits.map((c) => (
                  <li key={c.id}>
                    <Link to={`/chats/${c.id}`} className="conv-item">
                      <Avatar name={c.title} id={c.peerId ?? c.id} size={44} group={c.kind === "group"} />
                      <span className="conv-body"><span className="conv-name truncate"><Highlight text={c.title} q={q} /></span><span className="conv-preview truncate"><Highlight text={c.lastMessage?.text ?? ""} q={q} /></span></span>
                    </Link>
                  </li>))}
                </ul>
              )}
            </section>
          </>
        )}
        {state === "idle" && recents.length === 0 && q === "" && <Button variant="ghost" size="sm" onClick={() => navigate("/friends")}>Go to friends</Button>}
      </div>
    </div>
  );
}
