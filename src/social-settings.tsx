import { createSignal, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  APIError,
  fetchAuthSession,
  fetchCreatorSocial,
  fetchOwnedProfile,
  updateDiscoveryProfile,
  type AuthSession,
  type Profile,
} from "./api";
import { discoveryStyles as styles } from "./discover.stylex";

const sx = stylex.attrs;

export function SocialSettingsPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [profile, setProfile] = createSignal<Profile | null | undefined>();
  const [category, setCategory] = createSignal("");
  const [interests, setInterests] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [message, setMessage] = createSignal("");
  const [error, setError] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) {
        setProfile(null);
        return;
      }
      const owned = await fetchOwnedProfile();
      setProfile(owned);
      if (!owned) return;
      const social = await fetchCreatorSocial(owned.handle);
      if (social) {
        setCategory(social.category ?? "");
        setInterests(social.interests.join(", "));
      }
    } catch (reason) {
      setError(readableError(reason));
      setProfile(null);
    }
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const parsedInterests = interests()
        .split(/[\n,]/)
        .map((value) => value.trim())
        .filter(Boolean);
      const creator = await updateDiscoveryProfile({ category: category(), interests: parsedInterests });
      setCategory(creator.category ?? "");
      setInterests(creator.interests.join(", "));
      setMessage("Discovery profile saved.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <Show when={session()} fallback={<div {...sx(styles.empty)}>Loading discovery profile…</div>}>
        {(current) => (
          <Show
            when={current().authenticated}
            fallback={
              <div {...sx(styles.settingsCard)}>
                <div {...sx(styles.eyebrow)}>DISCOVERY PROFILE</div>
                <h1 {...sx(styles.settingsTitle)}>Sign in to join the network.</h1>
                <p {...sx(styles.settingsCopy)}>Your category and interests make your public Vuta easier to find.</p>
                <a {...sx(styles.button)} href="/signin">Sign in</a>
              </div>
            }
          >
            <Show
              when={profile()}
              fallback={
                <Show when={profile() === null} fallback={<div {...sx(styles.empty)}>Loading your Vuta…</div>}>
                  <div {...sx(styles.settingsCard)}>
                    <div {...sx(styles.eyebrow)}>DISCOVERY PROFILE</div>
                    <h1 {...sx(styles.settingsTitle)}>Claim your Vuta first.</h1>
                    <p {...sx(styles.settingsCopy)}>A public creator identity is required before you can follow people or appear in creator discovery.</p>
                    <a {...sx(styles.button)} href="/create">Claim your Vuta</a>
                  </div>
                </Show>
              }
            >
              {(item) => (
                <form {...sx(styles.settingsCard)} onSubmit={save}>
                  <div {...sx(styles.eyebrow)}>DISCOVERY PROFILE</div>
                  <h1 {...sx(styles.settingsTitle)}>Help people find @{item().handle}.</h1>
                  <p {...sx(styles.settingsCopy)}>Choose one broad category and up to eight specific interests. Interests are normalized and searchable across Vutame.</p>
                  <Show when={message()}><div {...sx(styles.notice)} role="status">{message()}</div></Show>
                  <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
                  <label {...sx(styles.field)}>
                    <span {...sx(styles.label)}>CATEGORY</span>
                    <input {...sx(styles.input)} value={category()} maxlength="40" onInput={(event) => setCategory(event.currentTarget.value)} placeholder="Engineering" />
                  </label>
                  <label {...sx(styles.field)}>
                    <span {...sx(styles.label)}>INTERESTS · UP TO 8</span>
                    <textarea {...sx(styles.input, styles.textarea)} value={interests()} onInput={(event) => setInterests(event.currentTarget.value)} placeholder="zig, distributed systems, game development" />
                  </label>
                  <div {...sx(styles.actions)}>
                    <button {...sx(styles.button)} type="submit" disabled={busy()}>{busy() ? "Saving…" : "Save discovery profile"}</button>
                    <a {...sx(styles.button, styles.secondaryButton)} href="/discover">Preview discovery</a>
                    <a {...sx(styles.button, styles.secondaryButton)} href="/settings">Account settings</a>
                  </div>
                </form>
              )}
            </Show>
          </Show>
        )}
      </Show>
    </section>
  );
}

function readableError(reason: unknown) {
  if (reason instanceof APIError) return reason.message;
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
