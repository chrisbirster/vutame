import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  APIError,
  createOwnedLink,
  fetchAppMeta,
  fetchAuthSession,
  fetchOwnedProfile,
  type AppMeta,
  type AuthSession,
  type Profile,
} from "./api";
import { LINK_KINDS, normalizeLinkKind } from "./link-kinds";
import { parseLinkImport, type ImportedLink } from "./link-import";
import { linkToolsStyles as styles } from "./link-tools.stylex";

const sx = stylex.attrs;

export function LinkToolsPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [profile, setProfile] = createSignal<Profile | null | undefined>();
  const [appMeta, setAppMeta] = createSignal<AppMeta | undefined>();
  const [source, setSource] = createSignal("");
  const [rows, setRows] = createSignal<ImportedLink[]>([]);
  const [parseErrors, setParseErrors] = createSignal<string[]>([]);
  const [busy, setBusy] = createSignal(false);
  const [message, setMessage] = createSignal("");
  const [error, setError] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const [current, meta] = await Promise.all([fetchAuthSession(), fetchAppMeta()]);
      setSession(current);
      setAppMeta(meta);
      if (current.authenticated) setProfile(await fetchOwnedProfile());
      else setProfile(null);
    } catch (reason) {
      setError(readableError(reason));
      setProfile(null);
    }
  }

  const canonicalURL = () => {
    const item = profile();
    if (!item) return "";
    const origin = (appMeta()?.profile_origin || "https://vuta.me").replace(/\/$/, "");
    return `${origin}/@${item.handle}`;
  };

  const qrURL = () => {
    const item = profile();
    return item ? `/api/v1/profiles/${encodeURIComponent(item.handle)}/qr.png` : "";
  };

  function parseSource() {
    const parsed = parseLinkImport(source());
    setRows(parsed.rows);
    setParseErrors(parsed.errors);
    setError("");
    setMessage(parsed.rows.length > 0 ? `Found ${parsed.rows.length} link${parsed.rows.length === 1 ? "" : "s"}.` : "No importable links found.");
  }

  function updateRow(index: number, patch: Partial<ImportedLink>) {
    setRows((current) => current.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row));
  }

  function removeRow(index: number) {
    setRows((current) => current.filter((_, rowIndex) => rowIndex !== index));
  }

  async function importRows() {
    if (!profile() || rows().length === 0) return;
    setBusy(true);
    setError("");
    setMessage("");
    let imported = 0;
    const failed: ImportedLink[] = [];
    for (const row of rows()) {
      try {
        await createOwnedLink({
          label: row.label,
          url: row.url,
          kind: row.kind,
          thumbnail_url: row.thumbnail_url,
          featured: false,
          visible_from: "",
          visible_until: "",
          is_active: true,
        });
        imported += 1;
      } catch (reason) {
        failed.push({ ...row, note: `Import failed: ${readableError(reason)}` });
      }
    }
    setRows(failed);
    setBusy(false);
    if (imported > 0) setMessage(`Imported ${imported} link${imported === 1 ? "" : "s"}. ${failed.length ? `${failed.length} need attention.` : "Your Vuta is updated."}`);
    if (imported === 0 && failed.length > 0) setError("No links were imported. Review the row messages and try again.");
  }

  async function copyProfileURL() {
    try {
      if (!navigator.clipboard) throw new Error("Clipboard access is unavailable in this browser.");
      await navigator.clipboard.writeText(canonicalURL());
      setMessage("Profile URL copied.");
      setError("");
    } catch (reason) {
      setError(readableError(reason));
    }
  }

  return (
    <section {...sx(styles.page)}>
      <Show when={session()} fallback={<div {...sx(styles.empty)}>Loading sharing tools…</div>}>
        {(current) => (
          <Show when={current().authenticated} fallback={<SignedOut />}>
            <Show when={profile() !== undefined} fallback={<div {...sx(styles.empty)}>Loading your Vuta…</div>}>
              <Show when={profile()} fallback={<ClaimFirst />}>
                {(item) => (
                  <>
                    <div {...sx(styles.heading)}>
                      <div>
                        <div {...sx(styles.eyebrow)}>SHARE & IMPORT</div>
                        <h1 {...sx(styles.title)}>Grow @{item().handle}.</h1>
                        <p {...sx(styles.copy)}>Bring over a saved link list and share your Vuta anywhere.</p>
                      </div>
                      <a {...sx(styles.button, styles.secondary)} href="/create">Back to dashboard</a>
                    </div>

                    <Show when={message()}><div {...sx(styles.message)} role="status" aria-live="polite">{message()}</div></Show>
                    <Show when={error()}><div {...sx(styles.message, styles.error)} role="alert">{error()}</div></Show>

                    <div {...sx(styles.workspace)}>
                      <div>
                        <div {...sx(styles.panel)}>
                          <h2 {...sx(styles.sectionTitle)}>Import a link list</h2>
                          <p {...sx(styles.sectionCopy)}>Paste CSV with label/title and URL columns, tab/pipe-separated rows, or one copied URL per line. Vutame imports at most 50 links at a time.</p>
                          <label {...sx(styles.field)}>
                            <span {...sx(styles.label)}>LINK LIST</span>
                            <textarea
                              {...sx(styles.input, styles.textarea)}
                              value={source()}
                              onInput={(event) => setSource(event.currentTarget.value)}
                              placeholder={'label,url,kind\nGitHub,https://github.com/you,github\nNewsletter | https://example.com/news'}
                            />
                          </label>
                          <div {...sx(styles.actions)}>
                            <button {...sx(styles.button)} type="button" disabled={busy()} onClick={parseSource}>Parse links</button>
                            <Show when={rows().length > 0}>
                              <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy()} onClick={() => void importRows()}>{busy() ? "Importing…" : `Import ${rows().length}`}</button>
                            </Show>
                          </div>
                          <Show when={parseErrors().length > 0}>
                            <ul {...sx(styles.errors)}><For each={parseErrors()}>{(entry) => <li>{entry}</li>}</For></ul>
                          </Show>
                        </div>

                        <Show when={rows().length > 0}>
                          <div {...sx(styles.panel)}>
                            <h2 {...sx(styles.sectionTitle)}>Review before import</h2>
                            <For each={rows()}>
                              {(row, index) => (
                                <div {...sx(styles.row)}>
                                  <div {...sx(styles.rowHeader)}>
                                    <strong>Link {index() + 1}</strong>
                                    <button {...sx(styles.button, styles.secondary)} type="button" disabled={busy()} onClick={() => removeRow(index())}>Remove</button>
                                  </div>
                                  <div {...sx(styles.grid)}>
                                    <label {...sx(styles.field)}>
                                      <span {...sx(styles.label)}>LABEL</span>
                                      <input {...sx(styles.input)} value={row.label} maxlength="100" onInput={(event) => updateRow(index(), { label: event.currentTarget.value })} />
                                    </label>
                                    <label {...sx(styles.field)}>
                                      <span {...sx(styles.label)}>KIND</span>
                                      <select {...sx(styles.input)} value={row.kind} onChange={(event) => updateRow(index(), { kind: normalizeLinkKind(event.currentTarget.value) })}>
                                        <For each={LINK_KINDS}>{(kind) => <option value={kind.id}>{kind.label}</option>}</For>
                                      </select>
                                    </label>
                                  </div>
                                  <label {...sx(styles.field)}>
                                    <span {...sx(styles.label)}>URL</span>
                                    <input {...sx(styles.input)} value={row.url} type="url" onInput={(event) => updateRow(index(), { url: event.currentTarget.value })} />
                                  </label>
                                  <label {...sx(styles.field)}>
                                    <span {...sx(styles.label)}>THUMBNAIL URL</span>
                                    <input {...sx(styles.input)} value={row.thumbnail_url} type="url" onInput={(event) => updateRow(index(), { thumbnail_url: event.currentTarget.value })} />
                                  </label>
                                  <Show when={row.note}><p {...sx(styles.note)}>{row.note}</p></Show>
                                </div>
                              )}
                            </For>
                          </div>
                        </Show>
                      </div>

                      <aside {...sx(styles.panel, styles.stickyPanel)} aria-label="Share your Vuta">
                        <h2 {...sx(styles.sectionTitle)}>Share your Vuta</h2>
                        <p {...sx(styles.canonical)}>{canonicalURL()}</p>
                        <img {...sx(styles.qr)} src={qrURL()} alt={`QR code for ${canonicalURL()}`} width="220" height="220" loading="lazy" decoding="async" />
                        <div {...sx(styles.actions)}>
                          <button {...sx(styles.button)} type="button" onClick={() => void copyProfileURL()}>Copy URL</button>
                          <a {...sx(styles.button, styles.secondary)} href={qrURL()} download={`vutame-${item().handle}-qr.png`}>Download QR</a>
                        </div>
                      </aside>
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

function SignedOut() {
  return <div {...sx(styles.panel, styles.empty)}><h1 {...sx(styles.title)}>Sign in to share or import.</h1><p {...sx(styles.copy)}>These tools update your Vutame account.</p><a {...sx(styles.button)} href="/signin">Sign in</a></div>;
}

function ClaimFirst() {
  return <div {...sx(styles.panel, styles.empty)}><h1 {...sx(styles.title)}>Claim your Vuta first.</h1><p {...sx(styles.copy)}>Once you have a public profile, you can import links and download its QR code.</p><a {...sx(styles.button)} href="/create">Claim your Vuta</a></div>;
}

function readableError(reason: unknown) {
  if (reason instanceof APIError) return reason.message;
  return reason instanceof Error ? reason.message : "Something went wrong. Please try again.";
}
