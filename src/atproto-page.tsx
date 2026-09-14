import { createSignal, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAuthSession, type AuthSession } from "./api";
import {
  fetchATProtoAccount,
  resolveATProtoIdentity,
  startATProtoOAuth,
  unlinkATProto,
  updateATProtoSettings,
  type ATProtoAccountState,
  type ATProtoIdentity,
} from "./atproto-api";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;

export function ATProtoPage() {
  const [session, setSession] = createSignal<AuthSession>();
  const [accountState, setAccountState] = createSignal<ATProtoAccountState>();
  const [identifier, setIdentifier] = createSignal("");
  const [resolved, setResolved] = createSignal<ATProtoIdentity>();
  const [conflictPolicy, setConflictPolicy] = createSignal<"vutame_wins" | "pds_wins">("vutame_wins");
  const [publishEnabled, setPublishEnabled] = createSignal(false);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) return;
      const state = await fetchATProtoAccount();
      applyAccountState(state);
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
      setMessage("AT Protocol preferences saved.");
    });
  }

  async function unlink() {
    await run(async () => {
      await unlinkATProto();
      applyAccountState({ linked: false });
      setResolved(undefined);
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
              when={accountState()?.linked ? accountState() : undefined}
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
              {(state) => (
                <div {...sx(styles.grid)}>
                  <section {...sx(styles.panel, styles.wide)}>
                    <h2 {...sx(styles.panelTitle)}>Linked identity</h2>
                    <div {...sx(styles.contact)}>
                      <div>
                        <strong>{state().linked ? state().account.handle || state().account.did : ""}</strong>
                        <div {...sx(styles.muted)}>{state().linked ? state().account.did : ""}</div>
                        <div {...sx(styles.muted)}>PDS · {state().linked ? state().account.pds_url : ""}</div>
                      </div>
                    </div>
                    <p {...sx(styles.help)}>
                      OAuth tokens and the DPoP private key are encrypted at rest. Unlinking removes Vutame's stored OAuth credentials and DID mapping; it does not delete anything from your PDS.
                    </p>
                  </section>

                  <section {...sx(styles.panel)}>
                    <h2 {...sx(styles.panelTitle)}>Portability preferences</h2>
                    <form {...sx(styles.form)} onSubmit={saveSettings}>
                      <label {...sx(styles.field)}>
                        <span {...sx(styles.label)}>CONFLICT POLICY</span>
                        <select {...sx(styles.input)} value={conflictPolicy()} onChange={(event) => setConflictPolicy(event.currentTarget.value as "vutame_wins" | "pds_wins")}>
                          <option value="vutame_wins">Vutame editor wins</option>
                          <option value="pds_wins">PDS records win</option>
                        </select>
                      </label>
                      <label {...sx(styles.checkRow)}>
                        <input {...sx(styles.checkbox)} type="checkbox" checked={publishEnabled()} onChange={(event) => setPublishEnabled(event.currentTarget.checked)} />
                        <span>Opt in to publishing public Vutame records to my PDS</span>
                      </label>
                      <p {...sx(styles.help)}>
                        This currently stores your publication preference. PDS writes are activated in M5 Slice 2 after token refresh, record sync, and conflict handling are complete.
                      </p>
                      <button {...sx(styles.button)} type="submit" disabled={busy()}>Save preferences</button>
                    </form>
                  </section>

                  <section {...sx(styles.panel)}>
                    <h2 {...sx(styles.panelTitle)}>Disconnect</h2>
                    <p {...sx(styles.help)}>Keep your Vutame profile while removing the linked AT identity and Vutame's stored OAuth credentials.</p>
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
