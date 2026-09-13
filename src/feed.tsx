import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAuthSession, type AuthSession } from "./api";
import {
  fetchFollowingActivity,
  fetchRecentActivity,
  fetchTrendingCreators,
  type ActivityEvent,
  type TrendingCreator,
} from "./activity-api";
import { feedStyles as styles } from "./feed.stylex";

const sx = stylex.attrs;
type FeedMode = "following" | "recent";

export function FeedPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [mode, setMode] = createSignal<FeedMode>("recent");
  const [events, setEvents] = createSignal<ActivityEvent[]>([]);
  const [nextCursor, setNextCursor] = createSignal("");
  const [trending, setTrending] = createSignal<TrendingCreator[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadingMore, setLoadingMore] = createSignal(false);
  const [error, setError] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      const initialMode: FeedMode = current.authenticated ? "following" : "recent";
      setMode(initialMode);
      const [page, trends] = await Promise.all([
        initialMode === "following" ? fetchFollowingActivity() : fetchRecentActivity(),
        fetchTrendingCreators(10),
      ]);
      setEvents(page.events);
      setNextCursor(page.next_cursor ?? "");
      setTrending(trends);
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoading(false);
    }
  }

  async function changeMode(next: FeedMode) {
    if (next === "following" && !session()?.authenticated) {
      window.location.assign(`/signin?next=${encodeURIComponent("/feed")}`);
      return;
    }
    if (next === mode()) return;
    setMode(next);
    setLoading(true);
    setError("");
    try {
      const page = next === "following" ? await fetchFollowingActivity() : await fetchRecentActivity();
      setEvents(page.events);
      setNextCursor(page.next_cursor ?? "");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoading(false);
    }
  }

  async function loadMore() {
    const cursor = nextCursor();
    if (!cursor) return;
    setLoadingMore(true);
    setError("");
    try {
      const page = mode() === "following"
        ? await fetchFollowingActivity(cursor)
        : await fetchRecentActivity(cursor);
      setEvents((current) => [...current, ...page.events]);
      setNextCursor(page.next_cursor ?? "");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoadingMore(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.heading)}>
        <div>
          <div {...sx(styles.eyebrow)}>ACTIVITY</div>
          <h1 {...sx(styles.title)}>What people are making.</h1>
        </div>
        <p {...sx(styles.intro)}>
          Follow creators for a focused feed, or browse recent public updates across Vutame.
        </p>
      </div>

      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <div {...sx(styles.layout)}>
        <main {...sx(styles.panel)}>
          <div {...sx(styles.tabs)} role="tablist" aria-label="Activity feed">
            <button
              {...sx(styles.button, mode() === "following" && styles.activeButton)}
              type="button"
              role="tab"
              aria-selected={mode() === "following" ? "true" : "false"}
              onClick={() => void changeMode("following")}
            >
              Following
            </button>
            <button
              {...sx(styles.button, mode() === "recent" && styles.activeButton)}
              type="button"
              role="tab"
              aria-selected={mode() === "recent" ? "true" : "false"}
              onClick={() => void changeMode("recent")}
            >
              Recent
            </button>
          </div>

          <Show
            when={!loading()}
            fallback={<div {...sx(styles.empty)}>Loading activity…</div>}
          >
            <Show
              when={events().length > 0}
              fallback={
                <div {...sx(styles.empty)}>
                  {mode() === "following"
                    ? "Your following feed is quiet. Discover creators and follow a few people."
                    : "No public activity yet."}
                </div>
              }
            >
              <For each={events()}>{(event) => <ActivityRow event={event} />}</For>
              <Show when={nextCursor()}>
                <button {...sx(styles.button, styles.loadMore)} type="button" disabled={loadingMore()} onClick={() => void loadMore()}>
                  {loadingMore() ? "Loading…" : "Load more"}
                </button>
              </Show>
            </Show>
          </Show>
        </main>

        <aside {...sx(styles.panel)} aria-label="Trending creators">
          <h2 {...sx(styles.sideTitle)}>Trending this week</h2>
          <p {...sx(styles.sideCopy)}>Score = 3 × public updates in the last 7 days + followers.</p>
          <Show when={trending().length > 0} fallback={<p {...sx(styles.sideCopy)}>Trending data will appear as the network gets active.</p>}>
            <For each={trending()}>
              {(creator, index) => (
                <div {...sx(styles.trend)}>
                  <span {...sx(styles.rank)}>#{index() + 1}</span>
                  <div {...sx(styles.trendIdentity)}>
                    <a {...sx(styles.trendName)} href={`/@${creator.handle}`}>{creator.display_name || `@${creator.handle}`}</a>
                    <span {...sx(styles.trendMeta)}>{creator.recent_activity} updates · {creator.follower_count} followers</span>
                  </div>
                  <span {...sx(styles.score)}>{creator.score}</span>
                </div>
              )}
            </For>
          </Show>
        </aside>
      </div>
    </section>
  );
}

function ActivityRow(props: { event: ActivityEvent }) {
  const initial = () => (props.event.display_name || props.event.handle).slice(0, 1).toUpperCase();
  const sentence = () => props.event.kind === "link_featured" ? "featured" : "updated their Vuta";
  return (
    <article {...sx(styles.event)}>
      <Show
        when={props.event.avatar_url}
        fallback={<div {...sx(styles.avatar)} aria-hidden="true">{initial()}</div>}
      >
        {(avatar) => <img {...sx(styles.avatar)} src={avatar()} alt="" width="48" height="48" loading="lazy" decoding="async" />}
      </Show>
      <div {...sx(styles.eventCopy)}>
        <p {...sx(styles.eventText)}>
          <a {...sx(styles.creator)} href={`/@${props.event.handle}`}>{props.event.display_name || `@${props.event.handle}`}</a>
          {` ${sentence()}`}
          <Show when={props.event.kind === "link_featured" && props.event.label}>
            {` `}<span {...sx(styles.label)}>{props.event.label}</span>
          </Show>
          .
        </p>
        <time {...sx(styles.time)} datetime={props.event.created_at}>{formatTime(props.event.created_at)}</time>
      </div>
    </article>
  );
}

function formatTime(value: string) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "Recently";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function readableError(reason: unknown) {
  return reason instanceof Error ? reason.message : "Could not load activity.";
}
