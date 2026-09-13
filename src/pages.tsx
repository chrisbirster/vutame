import { createEffect, createSignal, For, Show } from "solid-js";
import { useParams } from "@solidjs/router";
import * as stylex from "@stylexjs/stylex";
import {
  APIError,
  fetchAuthSession,
  fetchCreatorSocial,
  fetchFollowers,
  fetchFollowing,
  fetchProfile,
  followCreator,
  searchCreators,
  unfollowCreator,
  type AuthSession,
  type Creator,
  type Profile,
} from "./api";
import { discoveryStyles as network } from "./discover.stylex";
import { ProfileSurface } from "./profile-surface";
import { styles } from "./styles.stylex";

const sx = stylex.attrs;

function normalizeRouteHandle(value: string | undefined) {
  if (!value) return "";
  return decodeURIComponent(value).replace(/^@/, "").trim().toLowerCase();
}

function ProfilePreview(props: { handle?: string }) {
  const handle = () => props.handle ?? "chrisdontmiss";
  return (
    <div {...sx(styles.previewCard)}>
      <div {...sx(styles.avatar)}>V</div>
      <strong {...sx(styles.previewName)}>@{handle()}</strong>
      <p {...sx(styles.muted)}>Everything I make, share, and care about.</p>
      <div {...sx(styles.previewLink)}>Latest project <span>↗</span></div>
      <div {...sx(styles.previewLink)}>Find me elsewhere <span>↗</span></div>
      <div {...sx(styles.previewLink)}>What I'm into now <span>↗</span></div>
    </div>
  );
}

export function HomePage() {
  return (
    <section {...sx(styles.hero)}>
      <div {...sx(styles.heroCopy)}>
        <div {...sx(styles.eyebrow)}>YOUR INTERNET, ONE PROFILE</div>
        <h1 {...sx(styles.heroTitle)}>More than a link page. <span {...sx(styles.accent)}>A social home.</span></h1>
        <p {...sx(styles.heroBody)}>
          Put everything you make, sell, write, stream, and care about behind one short URL.
          Then discover people through the things they actually choose to share.
        </p>
        <div {...sx(styles.actions)}>
          <a {...sx(styles.primaryButton)} href="/create">Create your Vuta</a>
          <a {...sx(styles.secondaryButton)} href="/discover">Explore people</a>
        </div>
        <div {...sx(styles.domainNote)}>
          <strong>vutame.com</strong> is the network. <strong>vuta.me/@you</strong> is yours.
        </div>
      </div>
      <div {...sx(styles.heroPreview)}>
        <div {...sx(styles.previewChrome)}>
          <span>vuta.me/@chrisdontmiss</span>
          <span>•••</span>
        </div>
        <ProfilePreview />
      </div>
    </section>
  );
}

export function DiscoverPage() {
  const [creators, setCreators] = createSignal<Creator[]>([]);
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [query, setQuery] = createSignal("");
  const [category, setCategory] = createSignal("");
  const [interest, setInterest] = createSignal("");
  const [busyHandle, setBusyHandle] = createSignal("");
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const [current, items] = await Promise.all([fetchAuthSession(), searchCreators()]);
      setSession(current);
      setCreators(items);
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
      setCreators(await searchCreators({ q: query(), category: category(), interest: interest() }));
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setLoading(false);
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
          <p {...sx(network.intro)}>Search creators by handle, name, bio, category, or the interests they chose to make discoverable.</p>
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
      <Show when={!loading() || creators().length > 0} fallback={<div {...sx(network.empty)}>Loading creators…</div>}>
        <Show when={creators().length > 0} fallback={<div {...sx(network.empty)}>No creators matched those filters.</div>}>
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
        </Show>
      </Show>
    </section>
  );
}

function CreatorCard(props: { creator: Creator; authenticated: boolean; busy: boolean; onToggle?: () => void }) {
  const initial = () => (props.creator.display_name || props.creator.handle).slice(0, 1).toUpperCase();
  return (
    <article {...sx(network.card)}>
      <div {...sx(network.cardTop)}>
        <Show
          when={props.creator.avatar_url}
          fallback={<div {...sx(network.avatar)} aria-hidden="true">{initial()}</div>}
        >
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

export function CreatePage() {
  const [handle, setHandle] = createSignal("");

  const clean = () => handle().replace(/^@/, "").replace(/[^a-zA-Z0-9._-]/g, "").toLowerCase();
  const destination = () => clean() ? `/@${clean()}` : "/@yourname";

  return (
    <section {...sx(styles.section, styles.createSection)}>
      <div {...sx(styles.createCard)}>
        <div {...sx(styles.eyebrow)}>CLAIM YOUR VUTA</div>
        <h1 {...sx(styles.sectionTitle)}>Start with your name.</h1>
        <p {...sx(styles.sectionIntro)}>Choose the short identity you want to share everywhere.</p>
        <label {...sx(styles.inputLabel)} for="handle">Your handle</label>
        <div {...sx(styles.handleInput)}>
          <span>vuta.me/@</span>
          <input
            {...sx(styles.textInput)}
            id="handle"
            value={handle()}
            onInput={(event) => setHandle(event.currentTarget.value)}
            placeholder="yourname"
            autocomplete="off"
          />
        </div>
        <a {...sx(styles.primaryButton, styles.fullButton)} href={destination()}>
          Preview {clean() ? `@${clean()}` : "your profile"}
        </a>
      </div>
    </section>
  );
}

export function ProfilePage() {
  const params = useParams();
  const handle = () => normalizeRouteHandle(params.handle);
  const [profile, setProfile] = createSignal<Profile | null | undefined>(undefined);
  const [social, setSocial] = createSignal<Creator | null>(null);
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [followBusy, setFollowBusy] = createSignal(false);
  const [error, setError] = createSignal("");

  void fetchAuthSession().then(setSession).catch(() => setSession({ authenticated: false }));

  createEffect(handle, (current) => {
    if (!current) {
      setProfile(null);
      setSocial(null);
      return;
    }
    setProfile(undefined);
    setSocial(null);
    setError("");
    void Promise.all([fetchProfile(current), fetchCreatorSocial(current)])
      .then(([item, socialItem]) => {
        setProfile(item);
        setSocial(socialItem);
      })
      .catch((reason) => {
        setError(readableError(reason));
        setProfile(null);
      });
  });

  async function toggleFollow() {
    const item = social();
    if (!item) return;
    if (!session()?.authenticated) {
      window.location.assign(`/signin?next=${encodeURIComponent(`/@${handle()}`)}`);
      return;
    }
    setFollowBusy(true);
    setError("");
    try {
      if (item.viewer_follows) await unfollowCreator(item.handle);
      else await followCreator(item.handle);
      setSocial({
        ...item,
        viewer_follows: !item.viewer_follows,
        follower_count: Math.max(0, item.follower_count + (item.viewer_follows ? -1 : 1)),
      });
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setFollowBusy(false);
    }
  }

  const isOwnProfile = () => {
    const current = session();
    const item = profile();
    return current?.authenticated === true && item?.id === current.user.id;
  };

  return (
    <section {...sx(styles.profilePage)}>
      <Show when={!error()} fallback={<div {...sx(styles.emptyState)}>{error()}</div>}>
        <Show
          when={profile()}
          fallback={
            <Show
              when={profile() === null}
              fallback={<div {...sx(styles.emptyState)}>Loading @{handle()}…</div>}
            >
              <div {...sx(styles.emptyState)}>
                <div {...sx(styles.avatar)}>{handle().slice(0, 1).toUpperCase() || "V"}</div>
                <h1>@{handle()}</h1>
                <p>This Vuta is open.</p>
                <a {...sx(styles.primaryButton)} href="/create">Claim it</a>
              </div>
            </Show>
          }
        >
          {(item) => (
            <div {...sx(styles.publicProfile)}>
              <Show when={social()}>
                {(socialItem) => (
                  <div {...sx(network.socialBar)}>
                    <a {...sx(network.socialStat)} href={`/network/@${socialItem().handle}/followers`}><strong {...sx(network.statStrong)}>{socialItem().follower_count}</strong> followers</a>
                    <a {...sx(network.socialStat)} href={`/network/@${socialItem().handle}/following`}><strong {...sx(network.statStrong)}>{socialItem().following_count}</strong> following</a>
                    <Show when={!isOwnProfile()}>
                      <button {...sx(network.button, socialItem().viewer_follows && network.followingButton)} type="button" disabled={followBusy()} onClick={() => void toggleFollow()}>
                        {followBusy() ? "Working…" : socialItem().viewer_follows ? "Following" : session()?.authenticated ? "Follow" : "Sign in to follow"}
                      </button>
                    </Show>
                  </div>
                )}
              </Show>
              <ProfileSurface profile={item()} />
            </div>
          )}
        </Show>
      </Show>
    </section>
  );
}

export function FollowersPage() {
  return <ConnectionsPage mode="followers" />;
}

export function FollowingPage() {
  return <ConnectionsPage mode="following" />;
}

function ConnectionsPage(props: { mode: "followers" | "following" }) {
  const params = useParams();
  const handle = () => normalizeRouteHandle(params.handle);
  const [creators, setCreators] = createSignal<Creator[]>([]);
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(true);

  createEffect(handle, (current) => {
    if (!current) return;
    setLoading(true);
    setError("");
    const request = props.mode === "followers" ? fetchFollowers(current) : fetchFollowing(current);
    void request
      .then(setCreators)
      .catch((reason) => setError(readableError(reason)))
      .finally(() => setLoading(false));
  });

  return (
    <section {...sx(network.page)}>
      <div {...sx(network.heading)}>
        <div>
          <div {...sx(network.eyebrow)}>{props.mode.toUpperCase()}</div>
          <h1 {...sx(network.title)}>@{handle()}</h1>
        </div>
        <a {...sx(network.button, network.secondaryButton)} href={`/@${handle()}`}>Back to profile</a>
      </div>
      <Show when={error()}><div {...sx(network.notice, network.error)}>{error()}</div></Show>
      <Show when={!loading()} fallback={<div {...sx(network.empty)}>Loading {props.mode}…</div>}>
        <Show when={creators().length > 0} fallback={<div {...sx(network.empty)}>No {props.mode} yet.</div>}>
          <div {...sx(network.profileGrid)}>
            <For each={creators()}>{(creator) => <CreatorCard creator={creator} authenticated={false} busy={false} />}</For>
          </div>
        </Show>
      </Show>
    </section>
  );
}

export function NotFoundPage() {
  return (
    <section {...sx(styles.emptyState)}>
      <h1>Nothing lives here yet.</h1>
      <p>Try discovering a profile or claim your own.</p>
      <a {...sx(styles.primaryButton)} href="/discover">Discover</a>
    </section>
  );
}

function readableError(reason: unknown) {
  if (reason instanceof APIError) return reason.message;
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
