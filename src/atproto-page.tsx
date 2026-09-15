import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAuthSession, type AuthSession } from "./api";
import {
  fetchATProtoAccount,
  fetchATProtoSyncStatus,
  resolveATProtoIdentity,
  startATProtoOAuth,
  syncATProto,
  unlinkATProto,
  updateATProtoSettings,
  type ATProtoAccountState,
  type ATProtoIdentity,
  type ATProtoSyncReport,
  type ATProtoSyncStatus,
} from "./atproto-api";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;

export function ATProtoPage() {
  const [session, setSession] = createSignal<AuthSession>();
  const [accountState, setAccountState] = createSignal<ATProtoAccountState>();
  const [syncStatus, setSyncStatus] = createSignal<ATProtoSyncStatus>();
  const [syncReport, setSyncReport] = createSignal<ATProtoSyncReport>();
  const [identifier, setIdentifier] = createSignal("");
  const [resolved, setResolved] = createSignal<ATProtoIdentity>();
  const [conflictPolicy, setConflictPolicy] = createSignal<"vutame_wins" | "pds_wins">("vutame_wins");
  const [publishEnabled, setPublishEnabled] = createSignal(false);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  const linkedAccount = () => {
    const state = accountState();
    return state?.linked ? state.account : undefined;
  };

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) return;
      const state = await fetchATProtoAccount();
      applyAccountState(state);
      if (state.linked) setSyncStatus(await fetchATProtoSyncStatus());
      const params = new URLSearchParams(window.location.search);
      if (params.get("linked") === "1") setMessage("AT Protocol identity linked successfully.");
      if (params.get("error") === "oauth") setError("AT Protocol authorization could not be completed. You can try linking again.");
    } catch (reason) {
      setError(readable(reason));
    }
  }

  function applyAccountState(state: ATProtoAccountState) {
    setAccountState(state);
    if (state.linked) {
      setConflictPolicy(state.account.conflict_policy);
      setPublishEnabled(state.account.publish_enabled);
    }
  }

  async function resolve(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const identity = await resolveATProtoIdentity(identifier());
      setResolved(identity);
      setMessage(identity.handle
        ? `Verified ${identity.handle} ↔ ${identity.did}.`
        : `Resolved ${identity.did}. No bidirectionally verified handle is currently available.`);
    });
  }

  async function link() {
    await run(async () => {
      const started = await startATProtoOAuth(identifier());
      window.location.assign(started.authorization_url);
    });
  }

  async function saveSettings(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const account = await updateATProtoSettings({
        conflict_policy: conflictPolicy(),
        publish_enabled: publishEnabled(),
      });
      applyAccountState({ linked: true, account });
      setSyncStatus(await fetchATProtoSyncStatus());
      setMessage("AT Protocol preferences saved. Publishing changes only when you choose Sync now.");
    });
  }

  async function syncNow() {
    await run(async () => {
      const report = await syncATProto();
      setSyncReport(report);
      setSyncStatus(await fetchATProtoSyncStatus());
      setMessage(`PDS sync complete: ${report.published} published, ${report.deleted} deleted, ${report.conflicts.length} conflicts.`);
    });
  }

  async function unlink() {
    await run(async () => {
      await unlinkATProto();
      applyAccountState({ linked: false });
      setResolved(undefined);
      setSyncStatus(undefined);
      setSyncReport(undefined);
      setMessage("AT Protocol identity unlinked from Vutame.");
    });
  }

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
    } catch (reason) {
      setError(readable(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.heading)}>
        <div>
          <div {...sx(styles.eyebrow)}>AT PROTOCOL</div>
          <h1 {...sx(styles.title)}>Make your Vuta portable.</h1>
        </div>
        <p {...sx(styles.intro)}>
          Link an existing AT Protocol identity without replacing your Vutame account. Your DID is the stable network identity; a handle is shown only after both sides of handle verification agree.
        </p>
      </div>

      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <Show when={message()}><div {...sx(styles.notice, styles.success)}>{message()}</div></Show>

      <Show when={session() !== undefined} fallback={<div {...sx(styles.panel)}>Loading AT Protocol settings…</div>}>
        <Show when={session()?.authenticated} fallback={<div {...sx(styles.panel)}>Sign in to link an AT Protocol identity.</div>}>
          <Show when={accountState() !== undefined} fallback={<div {...sx(styles.panel)}>Loading linked identity…</div>}>
            <Show
              when={linkedAccount()}
              fallback={
                <div {...sx(styles.grid)}>
                  <section {...sx(styles.panel, styles.wide)}>
                    <h2 {...sx(styles.panelTitle)}>Link your AT identity</h2>
                    <p {...sx(styles.help)}>
                      Enter a handle such as <code>name.bsky.social</code> or a <code>did:plc:…</code> / hostname-level <code>did:web:…</code>. Vutame resolves the DID, verifies its PDS, then uses AT Protocol OAuth with PKCE, PAR, and DPoP.
                    </p>
                    <form {...sx(styles.form)} onSubmit={resolve}>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>AT HANDLE OR DID</span>
                        <input
                          {...sx(styles.input)}
                          value={identifier()}
                          onInput={(event) => { setIdentifier(event.currentTarget.value); setResolved(undefined); }}
                          placeholder="name.bsky.social"
                          autocomplete="off"
                        />
                      </label>
                      <div {...sx(styles.actions)}>
                        <button {...sx(styles.button)} type="submit" disabled={busy() || !identifier().trim()}>Resolve identity</button>
                        <button {...sx(styles.button)} type="button" disabled={busy() || !resolved()} onClick={() => void link()}>Authorize with AT Protocol</button>
                      </div>
                    </form>
                    <Show when={resolved()}>{(identity) => (
                      <div {...sx(styles.contact)}>
                        <div>
                          <strong>{identity().handle || identity().did}</strong>
                          <div {...sx(styles.muted)}>{identity().did}</div>
                          <div {...sx(styles.muted)}>PDS · {identity().pds_url}</div>
                        </div>
                      </div>
                    )}</Show>
                  </section>
                </div>
              }
            >
              {(account) => (
                <div {...sx(styles.grid)}>
                  <section {...sx(styles.panel, styles.wide)}>
                    <h2 {...sx(styles.panelTitle)}>Linked identity</h2>
                    <div {...sx(styles.contact)}>
                      <div>
                        <strong>{account().handle || account().did}</strong>
                        <div {...sx(styles.muted)}>{account().did}</div>
                        <div {...sx(styles.muted)}>PDS · {account().pds_url}</div>
                      </div>
                    </div>
                    <p {...sx(styles.help)}>
                      OAuth access/refresh tokens and the DPoP private key are encrypted at rest. Refresh tokens are rotated serially so concurrent syncs cannot reuse a single-use token.
                    </p>
                  </section>

                  <section {...sx(styles.panel)}>
                    <h2 {...sx(styles.panelTitle)}>Portability preferences</h2>
                    <form {...sx(styles.form)} onSubmit={saveSettings}>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>CONFLICT POLICY</span>
                        <select {...sx(styles.input)} value={conflictPolicy()} onChange={(event) => setConflictPolicy(event.currentTarget.value as "vutame_wins" | "pds_wins")}>
                          <option value="vutame_wins">Vutame editor wins</option>
                          <option value="pds_wins">PDS records win on divergence</option>
                        </select>
                      </label>
                      <label {...sx(styles.checkRow)}>
                        <input {...sx(styles.checkbox)} type="checkbox" checked={publishEnabled()} onChange={(event) => setPublishEnabled(event.currentTarget.checked)} />
                        <span>Opt in to publishing public Vutame records to my PDS</span>
                      </label>
                      <p {...sx(styles.help)}>
                        <strong>Vutame wins</strong> overwrites remote divergence with the current Vutame editor state. <strong>PDS wins</strong> preserves an externally changed/deleted PDS record and reports a conflict instead of overwriting it.
                      </p>
                      <button {...sx(styles.button)} type="submit" disabled={busy()}>Save preferences</button>
                    </form>
                  </section>

                  <section {...sx(styles.panel)}>
                    <h2 {...sx(styles.panelTitle)}>Publish to your PDS</h2>
                    <p {...sx(styles.help)}>
                      Sync publishes <code>com.vutame.profile/self</code> plus currently active/public <code>com.vutame.link</code> records. Disabled, future, and expired links are removed from Vutame-managed portable records.
                    </p>
                    <button {...sx(styles.button)} type="button" disabled={busy() || !account().publish_enabled} onClick={() => void syncNow()}>Sync now</button>
                    <Show when={!account().publish_enabled}><p {...sx(styles.help)}>Enable publication above before syncing.</p></Show>
                    <Show when={syncReport()?.conflicts.length}>{
                      <div {...sx(styles.notice, styles.error)}>
                        <strong>PDS conflicts preserved:</strong>
                        <For each={syncReport()?.conflicts || []}>{(conflict) => <div>{conflict.collection}/{conflict.rkey} — {conflict.reason}</div>}</For>
                      </div>
                    }</Show>
                  </section>

                  <section {...sx(styles.panel, styles.wide)}>
                    <h2 {...sx(styles.panelTitle)}>Managed portable records</h2>
                    <Show when={(syncStatus()?.records.length || 0) > 0} fallback={<p {...sx(styles.help)}>No Vutame-managed PDS records have been synchronized yet.</p>}>
                      <div {...sx(styles.contacts)}>
                        <For each={syncStatus()?.records || []}>{(record) => (
                          <div {...sx(styles.contact)}>
                            <div>
                              <strong>{record.collection}/{record.rkey}</strong>
                              <div {...sx(styles.muted)}>CID · {record.cid || "unknown"}</div>
                              <div {...sx(styles.muted)}>Synced · {new Date(record.synced_at).toLocaleString()}</div>
                            </div>
                          </div>
                        )}</For>
                      </div>
                    </Show>
                  </section>

                  <section {...sx(styles.panel)}>
                    <h2 {...sx(styles.panelTitle)}>Disconnect</h2>
                    <p {...sx(styles.help)}>Keep your Vutame profile while removing the linked AT identity and Vutame's stored OAuth credentials. Existing records in your PDS are left in your control.</p>
                    <button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void unlink()}>Unlink AT Protocol</button>
                  </section>
                </div>
              )}
            </Show>
          </Show>
        </Show>
      </Show>
    </section>
  );
}

function readable(reason: unknown) {
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
