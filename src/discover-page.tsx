import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  fetchAuthSession,
  followCreator,
  unfollowCreator,
  type AuthSession,
  type Creator,
} from "./api";
import { searchPortableProfiles, type PortableProfile } from "./atproto-api";
import { fetchDiscoveryPage } from "./discovery-api";
import { discoveryStyles as network } from "./discover.stylex";

const sx = stylex.attrs;

export function DiscoverPage() {
  const [creators, setCreators] = createSignal<Creator[]>([]);
  const [portable, setPortable] = createSignal<PortableProfile[]>([]);
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [query, setQuery] = createSignal("");
  const [category, setCategory] = createSignal("");
  const [interest, setInterest] = createSignal("");
  const [nextCursor, setNextCursor] = createSignal("");
  const [busyHandle, setBusyHandle] = createSignal("");
  const [loading, setLoading] = createSignal(true);
  const [loadingMore, setLoadingMore] = createSignal(false);
  const [error, setError] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const [current, page, portableProfiles] = await Promise.all([
        fetchAuthSession(),
        fetchDiscoveryPage(),
        loadPortable("", "", ""),
      ]);
      setSession(current);
      setCreators(page.creators);
      setPortable(portableProfiles);
      setNextCursor(page.next_cursor || "");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoading(false);
    }
  }

  async function runSearch(event?: SubmitEvent) {
    event?.preventDefault();
    setLoading(true);
    setError("");
    try {
      const [page, portableProfiles] = await Promise.all([
        fetchDiscoveryPage({ q: query(), category: category(), interest: interest() }),
        loadPortable(query(), category(), interest()),
      ]);
      setCreators(page.creators);
      setPortable(portableProfiles);
      setNextCursor(page.next_cursor || "");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoading(false);
    }
  }

  async function loadMore() {
    const cursor = nextCursor();
    if (!cursor || loadingMore()) return;
    setLoadingMore(true);
    setError("");
    try {
      const page = await fetchDiscoveryPage({
        q: query(),
        category: category(),
        interest: interest(),
        cursor,
      });
      setCreators((current) => {
        const seen = new Set(current.map((item) => item.handle));
        return [...current, ...page.creators.filter((item) => !seen.has(item.handle))];
      });
      setNextCursor(page.next_cursor || "");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoadingMore(false);
    }
  }

  async function toggleFollow(item: Creator) {
    const current = session();
    if (!current?.authenticated) {
      window.location.assign(`/signin?next=${encodeURIComponent("/discover")}`);
      return;
    }
    setBusyHandle(item.handle);
    setError("");
    try {
      if (item.viewer_follows) await unfollowCreator(item.handle);
      else await followCreator(item.handle);
      setCreators((items) => items.map((candidate) => candidate.handle === item.handle ? {
        ...candidate,
        viewer_follows: !item.viewer_follows,
        follower_count: Math.max(0, candidate.follower_count + (item.viewer_follows ? -1 : 1)),
      } : candidate));
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusyHandle("");
    }
  }

  return (
    <section {...sx(network.page)}>
      <div {...sx(network.heading)}>
        <div>
          <div {...sx(network.eyebrow)}>DISCOVER</div>
          <h1 {...sx(network.title)}>Find your people.</h1>
        </div>
        <div>
          <p {...sx(network.intro)}>Search Vutame creators plus portable <code>com.vutame.*</code> profiles discovered on AT Protocol.</p>
          <Show when={session()?.authenticated}>
            <div {...sx(network.actions)}><a {...sx(network.button, network.secondaryButton)} href="/settings/discovery">Edit discovery profile</a></div>
          </Show>
        </div>
      </div>

      <form {...sx(network.panel, network.searchGrid)} onSubmit={runSearch}>
        <label {...sx(network.field)}>
          <span {...sx(network.label)}>SEARCH</span>
          <input {...sx(network.input)} value={query()} onInput={(event) => setQuery(event.currentTarget.value)} placeholder="handle, name, bio, or interest" />
        </label>
        <label {...sx(network.field)}>
          <span {...sx(network.label)}>CATEGORY</span>
          <input {...sx(network.input)} value={category()} onInput={(event) => setCategory(event.currentTarget.value)} placeholder="Engineering" />
        </label>
        <label {...sx(network.field)}>
          <span {...sx(network.label)}>INTEREST</span>
          <input {...sx(network.input)} value={interest()} onInput={(event) => setInterest(event.currentTarget.value)} placeholder="zig" />
        </label>
        <button {...sx(network.button)} type="submit" disabled={loading()}>{loading() ? "Searching…" : "Search"}</button>
      </form>

      <Show when={error()}><div {...sx(network.notice, network.error)} role="alert">{error()}</div></Show>
      <Show when={!loading() || creators().length > 0 || portable().length > 0} fallback={<div {...sx(network.empty)}>Loading creators…</div>}>
        <Show when={creators().length > 0 || portable().length > 0} fallback={<div {...sx(network.empty)}>No creators matched those filters.</div>}>
          <Show when={creators().length > 0}>
            <div {...sx(network.profileGrid)}>
              <For each={creators()}>
                {(creator) => (
                  <CreatorCard
                    creator={creator}
                    authenticated={session()?.authenticated === true}
                    busy={busyHandle() === creator.handle}
                    onToggle={() => void toggleFollow(creator)}
                  />
                )}
              </For>
            </div>
            <Show when={nextCursor()}>
              <div {...sx(network.actions)}>
                <button {...sx(network.button, network.secondaryButton)} type="button" disabled={loadingMore()} onClick={() => void loadMore()}>
                  {loadingMore() ? "Loading…" : "Load more creators"}
                </button>
              </div>
            </Show>
          </Show>

          <Show when={portable().length > 0}>
            <div {...sx(network.heading)}>
              <div>
                <div {...sx(network.eyebrow)}>AT PROTOCOL</div>
                <h2 {...sx(network.title)}>Portable Vutas</h2>
              </div>
              <p {...sx(network.intro)}>Indexed from user-owned PDS records. DIDs are the stable identity; portable-only creators are not duplicated when that DID is already linked to a local Vutame account.</p>
            </div>
            <div {...sx(network.profileGrid)}><For each={portable()}>{(item) => <PortableCard profile={item} />}</For></div>
          </Show>
        </Show>
      </Show>
    </section>
  );
}

async function loadPortable(q: string, category: string, interest: string) {
  if (category.trim() || interest.trim()) return [];
  try {
    return await searchPortableProfiles(q, 20);
  } catch {
    // Portable discovery is additive. Local discovery remains useful when the
    // AT Protocol index is disabled or temporarily unavailable.
    return [];
  }
}

function PortableCard(props: { profile: PortableProfile }) {
  const initial = () => (props.profile.display_name || props.profile.handle || "A").slice(0, 1).toUpperCase();
  return (
    <article {...sx(network.card)}>
      <div {...sx(network.cardTop)}>
        <Show when={props.profile.avatar_url} fallback={<div {...sx(network.avatar)} aria-hidden="true">{initial()}</div>}>
          {(avatar) => <img {...sx(network.avatar)} src={avatar()} alt="" width="52" height="52" loading="lazy" decoding="async" />}
        </Show>
        <div {...sx(network.identity)}>
          <a {...sx(network.name)} href={`/at/${encodeURIComponent(props.profile.did)}`}>{props.profile.display_name || props.profile.handle || "Portable Vuta"}</a>
          <span {...sx(network.handle)}>{props.profile.handle ? `@${props.profile.handle} · ` : ""}AT Protocol</span>
        </div>
      </div>
      <p {...sx(network.bio)}>{props.profile.bio || "Portable Vutame profile."}</p>
      <div {...sx(network.tags)}><span {...sx(network.tag, network.category)}>PDS</span><span {...sx(network.tag)}>{props.profile.did}</span></div>
      <div {...sx(network.cardFooter)}><a {...sx(network.profileLink)} href={`/at/${encodeURIComponent(props.profile.did)}`}>View portable Vuta →</a></div>
    </article>
  );
}

function CreatorCard(props: { creator: Creator; authenticated: boolean; busy: boolean; onToggle?: () => void }) {
  const initial = () => (props.creator.display_name || props.creator.handle).slice(0, 1).toUpperCase();
  return (
    <article {...sx(network.card)}>
      <div {...sx(network.cardTop)}>
        <Show when={props.creator.avatar_url} fallback={<div {...sx(network.avatar)} aria-hidden="true">{initial()}</div>}>
          {(avatar) => <img {...sx(network.avatar)} src={avatar()} alt="" width="52" height="52" loading="lazy" decoding="async" />}
        </Show>
        <div {...sx(network.identity)}>
          <a {...sx(network.name)} href={`/@${props.creator.handle}`}>{props.creator.display_name || `@${props.creator.handle}`}</a>
          <span {...sx(network.handle)}>@{props.creator.handle}{props.creator.verified ? " · verified" : ""}</span>
        </div>
      </div>
      <p {...sx(network.bio)}>{props.creator.bio || "No bio yet."}</p>
      <Show when={props.creator.category || props.creator.interests.length > 0}>
        <div {...sx(network.tags)}>
          <Show when={props.creator.category}><span {...sx(network.tag, network.category)}>{props.creator.category}</span></Show>
          <For each={props.creator.interests.slice(0, 5)}>{(tag) => <span {...sx(network.tag)}>{tag}</span>}</For>
        </div>
      </Show>
      <div {...sx(network.stats)}>
        <span><strong {...sx(network.statStrong)}>{props.creator.follower_count}</strong> followers</span>
        <span><strong {...sx(network.statStrong)}>{props.creator.following_count}</strong> following</span>
      </div>
      <div {...sx(network.cardFooter)}>
        <a {...sx(network.profileLink)} href={`/@${props.creator.handle}`}>View Vuta →</a>
        <Show when={props.onToggle}>
          <button
            {...sx(network.button, props.creator.viewer_follows && network.followingButton)}
            type="button"
            disabled={props.busy}
            onClick={() => props.onToggle?.()}
          >
            {props.busy ? "Working…" : props.creator.viewer_follows ? "Following" : props.authenticated ? "Follow" : "Sign in to follow"}
          </button>
        </Show>
      </div>
    </article>
  );
}

function readableError(reason: unknown) {
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
