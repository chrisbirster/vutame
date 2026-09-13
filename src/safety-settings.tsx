import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  blockCreator,
  fetchBlocks,
  fetchMutes,
  fetchPrivacy,
  muteCreator,
  reportCreator,
  unblockCreator,
  unmuteCreator,
  updatePrivacy,
  type PrivacySettings,
  type SafetyRelation,
} from "./safety-api";
import { safetyStyles as styles } from "./safety-settings.stylex";

const sx = stylex.attrs;

export function SafetySettingsPage() {
  const [privacy, setPrivacy] = createSignal<PrivacySettings>({
    discoverable: true,
    activity_visible: true,
    allow_follows: true,
  });
  const [blocks, setBlocks] = createSignal<SafetyRelation[]>([]);
  const [mutes, setMutes] = createSignal<SafetyRelation[]>([]);
  const [blockHandle, setBlockHandle] = createSignal("");
  const [muteHandle, setMuteHandle] = createSignal("");
  const [reportHandle, setReportHandle] = createSignal("");
  const [reportReason, setReportReason] = createSignal("spam");
  const [reportDetail, setReportDetail] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    setBusy(true);
    setError("");
    try {
      const [currentPrivacy, currentBlocks, currentMutes] = await Promise.all([
        fetchPrivacy(),
        fetchBlocks(),
        fetchMutes(),
      ]);
      setPrivacy(currentPrivacy);
      setBlocks(currentBlocks);
      setMutes(currentMutes);
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function savePrivacy() {
    await runAction(async () => {
      setPrivacy(await updatePrivacy(privacy()));
      setMessage("Privacy settings saved.");
    });
  }

  async function addBlock(event: SubmitEvent) {
    event.preventDefault();
    const handle = blockHandle().trim();
    if (!handle) return;
    await runAction(async () => {
      await blockCreator(handle);
      setBlockHandle("");
      setBlocks(await fetchBlocks());
      setMessage("Creator blocked. Any follows between you were removed.");
    });
  }

  async function addMute(event: SubmitEvent) {
    event.preventDefault();
    const handle = muteHandle().trim();
    if (!handle) return;
    await runAction(async () => {
      await muteCreator(handle);
      setMuteHandle("");
      setMutes(await fetchMutes());
      setMessage("Creator muted. Their activity is hidden from your feeds.");
    });
  }

  async function submitReport(event: SubmitEvent) {
    event.preventDefault();
    const handle = reportHandle().trim();
    if (!handle) return;
    await runAction(async () => {
      await reportCreator(handle, reportReason(), reportDetail());
      setReportHandle("");
      setReportDetail("");
      setMessage("Report submitted for review.");
    });
  }

  async function runAction(action: () => Promise<void>) {
    if (busy()) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.eyebrow)}>SAFETY & PRIVACY</div>
      <h1 {...sx(styles.title)}>Control your network.</h1>
      <p {...sx(styles.intro)}>
        Choose how your Vuta appears in discovery, who can follow you, and who you want removed from your network or feeds.
        Blocking removes follows in both directions. Muting only hides a creator's activity from your feeds.
      </p>

      <Show when={message()}><div {...sx(styles.notice, styles.success)} role="status">{message()}</div></Show>
      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>

      <div {...sx(styles.grid)}>
        <section {...sx(styles.panel, styles.wide)} aria-labelledby="privacy-heading">
          <h2 {...sx(styles.panelTitle)} id="privacy-heading">Privacy</h2>
          <p {...sx(styles.help)}>These controls affect Vutame discovery and social activity. Your direct public Vuta URL remains available unless moderation removes it.</p>
          <PrivacyToggle
            label="Appear in discovery"
            description="Allow your creator card to appear in search and trending."
            checked={privacy().discoverable}
            onChange={(value) => setPrivacy((current) => ({ ...current, discoverable: value }))}
          />
          <PrivacyToggle
            label="Show public activity"
            description="Allow profile updates and newly featured links to appear in Recent and Following feeds."
            checked={privacy().activity_visible}
            onChange={(value) => setPrivacy((current) => ({ ...current, activity_visible: value }))}
          />
          <PrivacyToggle
            label="Allow follows"
            description="Allow new followers. Turning this off removes current inbound follows."
            checked={privacy().allow_follows}
            onChange={(value) => setPrivacy((current) => ({ ...current, allow_follows: value }))}
          />
          <button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void savePrivacy()}>
            {busy() ? "Saving…" : "Save privacy"}
          </button>
        </section>

        <section {...sx(styles.panel)} aria-labelledby="block-heading">
          <h2 {...sx(styles.panelTitle)} id="block-heading">Blocked creators</h2>
          <p {...sx(styles.help)}>Blocked creators cannot follow you or appear in your discovery/social surfaces, and existing follows are removed both ways.</p>
          <form {...sx(styles.form)} onSubmit={addBlock}>
            <input {...sx(styles.input)} value={blockHandle()} onInput={(event) => setBlockHandle(event.currentTarget.value)} placeholder="@handle" aria-label="Creator handle to block" />
            <button {...sx(styles.button, styles.dangerButton)} type="submit" disabled={busy()}>Block creator</button>
          </form>
          <RelationList
            items={blocks()}
            empty="No blocked creators."
            action="Unblock"
            onAction={(handle) => void runAction(async () => {
              await unblockCreator(handle);
              setBlocks(await fetchBlocks());
              setMessage(`@${handle} unblocked.`);
            })}
          />
        </section>

        <section {...sx(styles.panel)} aria-labelledby="mute-heading">
          <h2 {...sx(styles.panelTitle)} id="mute-heading">Muted creators</h2>
          <p {...sx(styles.help)}>Muted creators remain followable and visible, but their activity is removed from your feeds.</p>
          <form {...sx(styles.form)} onSubmit={addMute}>
            <input {...sx(styles.input)} value={muteHandle()} onInput={(event) => setMuteHandle(event.currentTarget.value)} placeholder="@handle" aria-label="Creator handle to mute" />
            <button {...sx(styles.button, styles.secondaryButton)} type="submit" disabled={busy()}>Mute creator</button>
          </form>
          <RelationList
            items={mutes()}
            empty="No muted creators."
            action="Unmute"
            onAction={(handle) => void runAction(async () => {
              await unmuteCreator(handle);
              setMutes(await fetchMutes());
              setMessage(`@${handle} unmuted.`);
            })}
          />
        </section>

        <section {...sx(styles.panel, styles.wide)} aria-labelledby="report-heading">
          <h2 {...sx(styles.panelTitle)} id="report-heading">Report a creator</h2>
          <p {...sx(styles.help)}>Reports create a durable moderation record. They do not automatically block the creator, so you can choose both actions independently.</p>
          <form {...sx(styles.form)} onSubmit={submitReport}>
            <input {...sx(styles.input)} value={reportHandle()} onInput={(event) => setReportHandle(event.currentTarget.value)} placeholder="@handle" aria-label="Creator handle to report" />
            <select {...sx(styles.select)} value={reportReason()} onChange={(event) => setReportReason(event.currentTarget.value)} aria-label="Report reason">
              <option value="spam">Spam</option>
              <option value="harassment">Harassment</option>
              <option value="impersonation">Impersonation</option>
              <option value="unsafe">Unsafe content or behavior</option>
              <option value="other">Other</option>
            </select>
            <textarea {...sx(styles.textarea)} value={reportDetail()} onInput={(event) => setReportDetail(event.currentTarget.value)} maxlength={1000} placeholder="Optional details" aria-label="Report details" />
            <button {...sx(styles.button, styles.dangerButton)} type="submit" disabled={busy()}>Submit report</button>
          </form>
        </section>
      </div>
    </section>
  );
}

function PrivacyToggle(props: { label: string; description: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label {...sx(styles.toggleRow)}>
      <span {...sx(styles.toggleCopy)}>
        <span {...sx(styles.toggleLabel)}>{props.label}</span>
        <span {...sx(styles.help)}>{props.description}</span>
      </span>
      <input {...sx(styles.checkbox)} type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />
    </label>
  );
}

function RelationList(props: { items: SafetyRelation[]; empty: string; action: string; onAction: (handle: string) => void }) {
  return (
    <Show when={props.items.length > 0} fallback={<p {...sx(styles.help)}>{props.empty}</p>}>
      <div {...sx(styles.relationList)}>
        <For each={props.items}>
          {(item) => (
            <div {...sx(styles.relation)}>
              <span {...sx(styles.identity)}>
                <strong>{item.display_name || `@${item.handle}`}</strong>
                <span {...sx(styles.handle)}>@{item.handle}</span>
              </span>
              <button {...sx(styles.button, styles.secondaryButton)} type="button" onClick={() => props.onAction(item.handle)}>{props.action}</button>
            </div>
          )}
        </For>
      </div>
    </Show>
  );
}

function readableError(reason: unknown) {
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
