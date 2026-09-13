import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  APIError,
  claimOwnedProfile,
  createOwnedLink,
  deleteOwnedLink,
  fetchAuthSession,
  fetchOwnedProfile,
  logoutAuth,
  reorderOwnedLinks,
  updateOwnedLink,
  updateOwnedProfile,
  type AuthSession,
  type Link,
  type Profile,
  type ProfileUpdate,
} from "./api";
import { LINK_KINDS, normalizeLinkKind } from "./link-kinds";
import { ProfileEditorPanel } from "./profile-editor-panel";
import { editorStyles as styles } from "./editor.stylex";

const sx = stylex.attrs;

export function EditorPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [profile, setProfile] = createSignal<Profile | null | undefined>();
  const [handle, setHandle] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) {
        setProfile(null);
        return;
      }
      setProfile(await fetchOwnedProfile());
    } catch (reason) {
      setError(readableError(reason));
      setProfile(null);
    }
  }

  async function refreshProfile() {
    const next = await fetchOwnedProfile();
    setProfile(next);
    return next;
  }

  async function claim(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const item = await claimOwnedProfile(handle());
      setProfile(item);
      setMessage(`@${item.handle} is yours.`);
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function saveProfile(input: ProfileUpdate) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const item = await updateOwnedProfile(input);
      setProfile(item);
      setMessage("Profile saved.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function addLink(event: SubmitEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const data = new FormData(form);
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await createOwnedLink({
        label: field(data, "label"),
        url: field(data, "url"),
        kind: normalizeLinkKind(field(data, "kind")),
        thumbnail_url: field(data, "thumbnail_url"),
        featured: checked(data, "featured"),
        visible_from: toRFC3339(field(data, "visible_from")),
        visible_until: toRFC3339(field(data, "visible_until")),
        is_active: true,
      });
      form.reset();
      await refreshProfile();
      setMessage("Link added.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function saveLink(event: SubmitEvent, link: Link) {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await updateOwnedLink(link.id, {
        label: field(data, "label"),
        url: field(data, "url"),
        kind: normalizeLinkKind(field(data, "kind")),
        thumbnail_url: field(data, "thumbnail_url"),
        featured: checked(data, "featured"),
        visible_from: toRFC3339(field(data, "visible_from")),
        visible_until: toRFC3339(field(data, "visible_until")),
        is_active: link.is_active,
      });
      await refreshProfile();
      setMessage("Link saved.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function toggleLink(link: Link) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await updateOwnedLink(link.id, {
        label: link.label,
        url: link.url,
        kind: link.kind,
        thumbnail_url: link.thumbnail_url ?? "",
        featured: link.featured,
        visible_from: link.visible_from ?? "",
        visible_until: link.visible_until ?? "",
        is_active: !link.is_active,
      });
      await refreshProfile();
      setMessage(link.is_active ? "Link hidden from your public Vuta." : "Link is enabled again.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function removeLink(link: Link) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await deleteOwnedLink(link.id);
      await refreshProfile();
      setMessage("Link deleted.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function moveLink(index: number, delta: number) {
    const current = profile();
    if (!current) return;
    const destination = index + delta;
    if (destination < 0 || destination >= current.links.length) return;
    const ids = current.links.map((link) => link.id);
    [ids[index], ids[destination]] = [ids[destination], ids[index]];
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await reorderOwnedLinks(ids);
      await refreshProfile();
      setMessage("Link order updated.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <Show when={session()} fallback={<div {...sx(styles.empty)}>Loading your Vuta…</div>}>
        {(current) => (
          <Show
            when={current().authenticated}
            fallback={
              <div {...sx(styles.panel, styles.narrow, styles.empty)}>
                <div {...sx(styles.eyebrow)}>CREATOR DASHBOARD</div>
                <h1 {...sx(styles.title)}>Sign in to claim your Vuta.</h1>
                <p {...sx(styles.copy)}>Your profile, links, and ordering are tied to your verified email account.</p>
                <a {...sx(styles.button)} href="/signin">Sign in with email</a>
              </div>
            }
          >
            <Show when={profile() !== undefined} fallback={<div {...sx(styles.empty)}>Loading editor…</div>}>
              <Show
                when={profile()}
                fallback={
                  <div {...sx(styles.panel, styles.narrow)}>
                    <div {...sx(styles.eyebrow)}>CLAIM YOUR VUTA</div>
                    <h1 {...sx(styles.title)}>Choose your public name.</h1>
                    <p {...sx(styles.copy)}>This becomes your short public URL. Handles are unique and reserved names cannot be claimed.</p>
                    <Show when={error()}><div {...sx(styles.message, styles.error)}>{error()}</div></Show>
                    <form onSubmit={claim}>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>VUTA HANDLE</span>
                        <input {...sx(styles.input)} value={handle()} onInput={(event) => setHandle(event.currentTarget.value)} placeholder="yourname" autocomplete="off" required />
                      </label>
                      <button {...sx(styles.button)} type="submit" disabled={busy()}>{busy() ? "Claiming…" : `Claim ${handle().trim() ? `@${handle().replace(/^@/, "")}` : "your name"}`}</button>
                    </form>
                  </div>
                }
              >
                {(item) => (
                  <>
                    <div {...sx(styles.topbar)}>
                      <div>
                        <div {...sx(styles.eyebrow)}>CREATOR DASHBOARD</div>
                        <h1 {...sx(styles.title)}>@{item().handle}</h1>
                        <p {...sx(styles.copy)}>Design the public identity behind <strong>vuta.me/@{item().handle}</strong>.</p>
                      </div>
                      <div {...sx(styles.actions)}>
                        <a {...sx(styles.button, styles.secondary)} href={`/@${item().handle}`}>View public profile</a>
                        <a {...sx(styles.button, styles.secondary)} href="/settings">Account settings</a>
                      </div>
                    </div>

                    <Show when={message()}><div {...sx(styles.message)}>{message()}</div></Show>
                    <Show when={error()}><div {...sx(styles.message, styles.error)}>{error()}</div></Show>

                    <ProfileEditorPanel profile={item()} busy={busy()} onSave={saveProfile} />

                    <form {...sx(styles.panel)} onSubmit={addLink}>
                      <h2 {...sx(styles.sectionTitle)}>Add link</h2>
                      <p {...sx(styles.sectionCopy)}>Create a normal or featured link. Optional start/end times control when it appears publicly.</p>
                      <div {...sx(styles.grid)}>
                        <label {...sx(styles.field)}>
                          <span {...sx(styles.label)}>LABEL</span>
                          <input {...sx(styles.input)} name="label" maxlength="100" required placeholder="My latest project" />
                        </label>
                        <label {...sx(styles.field)}>
                          <span {...sx(styles.label)}>KIND</span>
                          <select {...sx(styles.input)} name="kind">
                            <For each={LINK_KINDS}>{(kind) => <option value={kind.id}>{kind.label}</option>}</For>
                          </select>
                        </label>
                      </div>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>URL</span>
                        <input {...sx(styles.input)} name="url" type="url" required placeholder="https://example.com" />
                      </label>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>THUMBNAIL URL <span {...sx(styles.optional)}>OPTIONAL</span></span>
                        <input {...sx(styles.input)} name="thumbnail_url" type="url" placeholder="https://example.com/card.jpg" />
                      </label>
                      <label {...sx(styles.checkRow)}>
                        <input type="checkbox" name="featured" />
                        <span><strong>Feature this link</strong><small>Featured links stay above normal links while preserving manual order.</small></span>
                      </label>
                      <div {...sx(styles.grid)}>
                        <label {...sx(styles.field)}>
                          <span {...sx(styles.label)}>VISIBLE FROM <span {...sx(styles.optional)}>OPTIONAL</span></span>
                          <input {...sx(styles.input)} name="visible_from" type="datetime-local" />
                        </label>
                        <label {...sx(styles.field)}>
                          <span {...sx(styles.label)}>VISIBLE UNTIL <span {...sx(styles.optional)}>OPTIONAL</span></span>
                          <input {...sx(styles.input)} name="visible_until" type="datetime-local" />
                        </label>
                      </div>
                      <button {...sx(styles.button)} type="submit" disabled={busy()}>Add link</button>
                    </form>

                    <div {...sx(styles.panel)}>
                      <h2 {...sx(styles.sectionTitle)}>Links</h2>
                      <p {...sx(styles.sectionCopy)}>Owned links stay visible here even while scheduled or expired. Public profiles only show enabled links inside their visibility window.</p>
                      <Show when={item().links.length > 0} fallback={<p {...sx(styles.copy)}>No links yet. Add your first one above.</p>}>
                        <For each={item().links}>
                          {(link, index) => (
                            <form {...sx(styles.linkCard)} onSubmit={(event) => saveLink(event, link)}>
                              <div {...sx(styles.linkHeader)}>
                                <strong>{index() + 1}. {link.label}</strong>
                                <div {...sx(styles.actions)}>
                                  <Show when={link.featured}><span {...sx(styles.badge)}>FEATURED</span></Show>
                                  <span {...sx(styles.badge, linkStatus(link) !== "PUBLIC" && styles.mutedBadge)}>{linkStatus(link)}</span>
                                </div>
                              </div>
                              <div {...sx(styles.grid)}>
                                <label {...sx(styles.field)}>
                                  <span {...sx(styles.label)}>LABEL</span>
                                  <input {...sx(styles.input)} name="label" value={link.label} maxlength="100" required />
                                </label>
                                <label {...sx(styles.field)}>
                                  <span {...sx(styles.label)}>KIND</span>
                                  <select {...sx(styles.input)} name="kind" value={link.kind}>
                                    <For each={LINK_KINDS}>{(kind) => <option value={kind.id}>{kind.label}</option>}</For>
                                  </select>
                                </label>
                              </div>
                              <label {...sx(styles.field)}>
                                <span {...sx(styles.label)}>URL</span>
                                <input {...sx(styles.input)} name="url" type="url" value={link.url} required />
                              </label>
                              <label {...sx(styles.field)}>
                                <span {...sx(styles.label)}>THUMBNAIL URL <span {...sx(styles.optional)}>OPTIONAL</span></span>
                                <input {...sx(styles.input)} name="thumbnail_url" type="url" value={link.thumbnail_url ?? ""} placeholder="https://example.com/card.jpg" />
                              </label>
                              <label {...sx(styles.checkRow)}>
                                <input type="checkbox" name="featured" checked={link.featured} />
                                <span><strong>Featured</strong><small>Show this ahead of non-featured links when it is publicly visible.</small></span>
                              </label>
                              <div {...sx(styles.grid)}>
                                <label {...sx(styles.field)}>
                                  <span {...sx(styles.label)}>VISIBLE FROM <span {...sx(styles.optional)}>OPTIONAL</span></span>
                                  <input {...sx(styles.input)} name="visible_from" type="datetime-local" value={toLocalDateTime(link.visible_from)} />
                                </label>
                                <label {...sx(styles.field)}>
                                  <span {...sx(styles.label)}>VISIBLE UNTIL <span {...sx(styles.optional)}>OPTIONAL</span></span>
                                  <input {...sx(styles.input)} name="visible_until" type="datetime-local" value={toLocalDateTime(link.visible_until)} />
                                </label>
                              </div>
                              <div {...sx(styles.actions)}>
                                <button {...sx(styles.button)} type="submit" disabled={busy()}>Save</button>
                                <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy() || index() === 0} onClick={() => void moveLink(index(), -1)}>↑ Up</button>
                                <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy() || index() === item().links.length - 1} onClick={() => void moveLink(index(), 1)}>↓ Down</button>
                                <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy()} onClick={() => void toggleLink(link)}>{link.is_active ? "Disable" : "Enable"}</button>
                                <button {...sx(styles.button, styles.danger)} type="button" disabled={busy()} onClick={() => void removeLink(link)}>Delete</button>
                              </div>
                            </form>
                          )}
                        </For>
                      </Show>
                    </div>
                  </>
                )}
              </Show>
            </Show>
          </Show>
        )}
      </Show>
    </section>
  );
}

export function SettingsPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [profile, setProfile] = createSignal<Profile | null | undefined>();
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");

  const account = () => {
    const current = session();
    return current?.authenticated ? current.user : undefined;
  };

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (current.authenticated) setProfile(await fetchOwnedProfile());
      else setProfile(null);
    } catch (reason) {
      setError(readableError(reason));
    }
  }

  async function signOut() {
    setBusy(true);
    try {
      await logoutAuth();
      window.location.assign("/signin");
    } catch (reason) {
      setError(readableError(reason));
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.panel, styles.narrow)}>
        <div {...sx(styles.eyebrow)}>ACCOUNT SETTINGS</div>
        <h1 {...sx(styles.title)}>Your Vutame account.</h1>
        <Show when={error()}><div {...sx(styles.message, styles.error)}>{error()}</div></Show>
        <Show when={session() !== undefined} fallback={<p {...sx(styles.copy)}>Loading account…</p>}>
          <Show when={account()} fallback={<p {...sx(styles.copy)}>You are signed out. <a href="/signin">Sign in</a> to manage your account.</p>}>
            {(user) => (
              <>
                <div {...sx(styles.accountRow)}><span {...sx(styles.label)}>EMAIL</span><span {...sx(styles.value)}>{user().email}</span></div>
                <div {...sx(styles.accountRow)}><span {...sx(styles.label)}>EMAIL VERIFIED</span><span {...sx(styles.value)}>{user().email_verified ? "Yes" : "No"}</span></div>
                <div {...sx(styles.accountRow)}><span {...sx(styles.label)}>PUBLIC VUTA</span><span {...sx(styles.value)}>{profile() ? `vuta.me/@${profile()!.handle}` : "Not claimed yet"}</span></div>
                <div {...sx(styles.actions)}>
                  <a {...sx(styles.button)} href="/create">{profile() ? "Open creator dashboard" : "Claim your Vuta"}</a>
                  <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy()} onClick={() => void signOut()}>{busy() ? "Signing out…" : "Sign out"}</button>
                </div>
              </>
            )}
          </Show>
        </Show>
      </div>
    </section>
  );
}

function field(data: FormData, name: string) {
  const value = data.get(name);
  return typeof value === "string" ? value : "";
}

function checked(data: FormData, name: string) {
  return data.has(name);
}

function toRFC3339(value: string) {
  const clean = value.trim();
  if (!clean) return "";
  const date = new Date(clean);
  return Number.isNaN(date.getTime()) ? clean : date.toISOString();
}

function toLocalDateTime(value: string | undefined) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function linkStatus(link: Link) {
  if (!link.is_active) return "DISABLED";
  const now = Date.now();
  if (link.visible_from && now < new Date(link.visible_from).getTime()) return "SCHEDULED";
  if (link.visible_until && now >= new Date(link.visible_until).getTime()) return "EXPIRED";
  return "PUBLIC";
}

function readableError(reason: unknown) {
  if (reason instanceof APIError) {
    if (reason.status === 401) return "Your session has expired. Sign in again to continue.";
    if (reason.status === 409) return reason.message;
    return reason.message;
  }
  return reason instanceof Error ? reason.message : "Something went wrong. Please try again.";
}
