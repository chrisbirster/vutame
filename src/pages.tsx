import { createEffect, createSignal, For, Show } from "solid-js";
import { useParams } from "@solidjs/router";
import * as stylex from "@stylexjs/stylex";
import { fetchDiscover, fetchProfile, type Profile } from "./api";
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
  const [profiles, setProfiles] = createSignal<Profile[]>([]);
  const [error, setError] = createSignal("");

  void fetchDiscover().then(setProfiles).catch((reason) => {
    setError(reason instanceof Error ? reason.message : "Could not load profiles.");
  });

  return (
    <section {...sx(styles.section)}>
      <div {...sx(styles.sectionHeading)}>
        <div>
          <div {...sx(styles.eyebrow)}>DISCOVER</div>
          <h1 {...sx(styles.sectionTitle)}>People, not landing pages.</h1>
        </div>
        <p {...sx(styles.sectionIntro)}>A creator directory that can grow into a real social graph.</p>
      </div>
      <Show when={!error()} fallback={<p {...sx(styles.muted)}>{error()}</p>}>
        <div {...sx(styles.profileGrid)}>
          <For each={profiles()}>
            {(profile) => (
              <a {...sx(styles.discoveryCard)} href={`/@${profile.handle}`}>
                <div {...sx(styles.avatar, styles.smallAvatar)}>{profile.handle.slice(0, 1).toUpperCase()}</div>
                <strong>@{profile.handle}</strong>
                <p {...sx(styles.muted)}>{profile.bio}</p>
                <span {...sx(styles.cardMeta)}>{profile.links.length} links · view profile →</span>
              </a>
            )}
          </For>
        </div>
      </Show>
    </section>
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
        <p {...sx(styles.sectionIntro)}>
          Accounts and persistence come next. This first slice proves the routing, profile surface, and product shape.
        </p>
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
  const [error, setError] = createSignal("");

  createEffect(handle, (current) => {
    if (!current) {
      setProfile(null);
      return;
    }
    setProfile(undefined);
    setError("");
    void fetchProfile(current)
      .then(setProfile)
      .catch((reason) => {
        setError(reason instanceof Error ? reason.message : "Could not load profile.");
        setProfile(null);
      });
  });

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
            <article {...sx(styles.publicProfile)}>
              <div {...sx(styles.avatar, styles.profileAvatar)}>{item().handle.slice(0, 1).toUpperCase()}</div>
              <h1 {...sx(styles.profileTitle)}>{item().display_name}</h1>
              <p {...sx(styles.profileBio)}>{item().bio}</p>
              <div {...sx(styles.linkStack)}>
                <For each={item().links}>
                  {(link) => (
                    <a {...sx(styles.publicLink)} href={link.url} target="_blank" rel="noreferrer">
                      <span>{link.label}</span><span>↗</span>
                    </a>
                  )}
                </For>
              </div>
              <a {...sx(styles.vutameBadge)} href="/">Made on vutame</a>
            </article>
          )}
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
