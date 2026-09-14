import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAppMeta, fetchAuthSession, fetchOwnedProfile, type AppMeta, type AuthSession, type Profile } from "./api";
import {
  fetchContacts,
  fetchDataSettings,
  fetchOwnedContactBlock,
  updateDataSettings,
  updateOwnedContactBlock,
  type ContactBlock,
  type ContactSubmission,
  type DataSettings,
} from "./growth-api";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;
const retentionOptions = [30, 90, 365] as const;

export function GrowthPage() {
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [profile, setProfile] = createSignal<Profile | null>(null);
  const [meta, setMeta] = createSignal<AppMeta | null>(null);
  const [block, setBlock] = createSignal<ContactBlock | null>(null);
  const [settings, setSettings] = createSignal<DataSettings | null>(null);
  const [contacts, setContacts] = createSignal<ContactSubmission[]>([]);
  const [source, setSource] = createSignal("");
  const [medium, setMedium] = createSignal("");
  const [campaign, setCampaign] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) return;
      const [owned, appMeta, contactBlock, retention, recent] = await Promise.all([
        fetchOwnedProfile(),
        fetchAppMeta(),
        fetchOwnedContactBlock(),
        fetchDataSettings(),
        fetchContacts(),
      ]);
      setProfile(owned);
      setMeta(appMeta);
      setBlock(contactBlock);
      setSettings(retention);
      setContacts(recent);
    } catch (reason) {
      setError(readableError(reason));
    }
  }

  const campaignURL = () => {
    const owned = profile();
    const appMeta = meta();
    if (!owned || !appMeta) return "";
    const url = new URL(`${appMeta.profile_origin}/@${owned.handle}`);
    if (source().trim()) url.searchParams.set("utm_source", source().trim());
    if (medium().trim()) url.searchParams.set("utm_medium", medium().trim());
    if (campaign().trim()) url.searchParams.set("utm_campaign", campaign().trim());
    return url.toString();
  };

  async function copyCampaignURL() {
    const value = campaignURL();
    if (!value) return;
    await navigator.clipboard.writeText(value);
    setMessage("Campaign URL copied.");
  }

  async function saveBlock(event: SubmitEvent) {
    event.preventDefault();
    const current = block();
    if (!current) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      setBlock(await updateOwnedContactBlock(current));
      setMessage("Contact capture settings saved.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function saveRetention(event: SubmitEvent) {
    event.preventDefault();
    const current = settings();
    if (!current) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      setSettings(await updateDataSettings(current));
      setContacts(await fetchContacts());
      setMessage("Retention settings saved and expired data purged.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.heading)}>
        <div>
          <div {...sx(styles.eyebrow)}>GROWTH TOOLS</div>
          <h1 {...sx(styles.title)}>Grow without creepy tracking.</h1>
        </div>
        <p {...sx(styles.intro)}>Build attributable profile URLs, collect explicitly consented emails, and decide how long Vutame keeps your analytics and contact data.</p>
      </div>

      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <Show when={message()}><div {...sx(styles.notice, styles.success)}>{message()}</div></Show>

      <Show when={session() !== undefined} fallback={<div {...sx(styles.panel)}>Loading growth tools…</div>}>
        <Show when={session()?.authenticated} fallback={<div {...sx(styles.panel)}>Sign in to use creator growth tools.</div>}>
          <Show when={profile()} fallback={<div {...sx(styles.panel)}>Claim your Vuta before using growth tools.</div>}>
            <div {...sx(styles.grid)}>
              <section {...sx(styles.panel, styles.wide)}>
                <h2 {...sx(styles.panelTitle)}>Campaign URL builder</h2>
                <p {...sx(styles.help)}>Generate a shareable Vuta URL. Vutame stores only the bounded <code>utm_campaign</code> label in aggregate analytics; source and medium remain in the URL but are not persisted as visitor fields.</p>
                <div {...sx(styles.row)}>
                  <label {...sx(styles.field)}><span {...sx(styles.label)}>SOURCE</span><input {...sx(styles.input)} value={source()} onInput={(event) => setSource(event.currentTarget.value)} placeholder="newsletter" /></label>
                  <label {...sx(styles.field)}><span {...sx(styles.label)}>MEDIUM</span><input {...sx(styles.input)} value={medium()} onInput={(event) => setMedium(event.currentTarget.value)} placeholder="email" /></label>
                  <label {...sx(styles.field)}><span {...sx(styles.label)}>CAMPAIGN</span><input {...sx(styles.input)} maxlength="80" value={campaign()} onInput={(event) => setCampaign(event.currentTarget.value)} placeholder="fall-launch" /></label>
                </div>
                <code {...sx(styles.generated)}>{campaignURL()}</code>
                <div {...sx(styles.actions)}><button {...sx(styles.button)} type="button" onClick={() => void copyCampaignURL()}>Copy campaign URL</button></div>
              </section>

              <Show when={block()}>{(currentBlock) => (
                <form {...sx(styles.panel)} onSubmit={saveBlock}>
                  <h2 {...sx(styles.panelTitle)}>Contact capture</h2>
                  <p {...sx(styles.help)}>Visitors must actively check the consent box. Vutame snapshots the exact consent text alongside each collected email.</p>
                  <label {...sx(styles.checkRow)}><input {...sx(styles.checkbox)} type="checkbox" checked={currentBlock().enabled} onChange={(event) => setBlock({ ...currentBlock(), enabled: event.currentTarget.checked })} /><span>Enable email capture on my public Vuta</span></label>
                  <div {...sx(styles.form)}>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>HEADING</span><input {...sx(styles.input)} maxlength="80" value={currentBlock().heading} onInput={(event) => setBlock({ ...currentBlock(), heading: event.currentTarget.value })} /></label>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>DESCRIPTION</span><textarea {...sx(styles.textarea)} maxlength="240" value={currentBlock().description} onInput={(event) => setBlock({ ...currentBlock(), description: event.currentTarget.value })} /></label>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>CONSENT TEXT</span><textarea {...sx(styles.textarea)} maxlength="320" value={currentBlock().consent_text} onInput={(event) => setBlock({ ...currentBlock(), consent_text: event.currentTarget.value })} /></label>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>BUTTON LABEL</span><input {...sx(styles.input)} maxlength="40" value={currentBlock().button_label} onInput={(event) => setBlock({ ...currentBlock(), button_label: event.currentTarget.value })} /></label>
                    <button {...sx(styles.button)} type="submit" disabled={busy()}>{busy() ? "Saving…" : "Save contact block"}</button>
                  </div>
                </form>
              )}</Show>

              <Show when={settings()}>{(retention) => (
                <form {...sx(styles.panel)} onSubmit={saveRetention}>
                  <h2 {...sx(styles.panelTitle)}>Data retention</h2>
                  <p {...sx(styles.help)}>Shortening either setting immediately deletes older rows. New events are also purged opportunistically as data arrives.</p>
                  <div {...sx(styles.form)}>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>ANALYTICS</span><select {...sx(styles.input)} value={String(retention().analytics_retention_days)} onChange={(event) => setSettings({ ...retention(), analytics_retention_days: Number(event.currentTarget.value) })}><For each={retentionOptions}>{(days) => <option value={String(days)}>{days} days</option>}</For></select></label>
                    <label {...sx(styles.field)}><span {...sx(styles.label)}>CONTACTS</span><select {...sx(styles.input)} value={String(retention().contact_retention_days)} onChange={(event) => setSettings({ ...retention(), contact_retention_days: Number(event.currentTarget.value) })}><For each={retentionOptions}>{(days) => <option value={String(days)}>{days} days</option>}</For></select></label>
                    <button {...sx(styles.button)} type="submit" disabled={busy()}>{busy() ? "Saving…" : "Save retention"}</button>
                  </div>
                </form>
              )}</Show>

              <section {...sx(styles.panel, styles.wide)}>
                <h2 {...sx(styles.panelTitle)}>Recent consented contacts</h2>
                <p {...sx(styles.help)}>Only the creator who owns this Vuta can read these addresses. Campaign labels are shown when the signup arrived through an attributable campaign URL.</p>
                <Show when={contacts().length > 0} fallback={<p {...sx(styles.help)}>No contacts collected yet.</p>}>
                  <div {...sx(styles.contacts)}><For each={contacts()}>{(contact) => <div {...sx(styles.contact)}><strong>{contact.email}</strong><span {...sx(styles.muted)}>{contact.campaign || "Unattributed"}</span><span {...sx(styles.muted)}>{new Date(contact.created_at).toLocaleDateString()}</span></div>}</For></div>
                </Show>
              </section>
            </div>
          </Show>
        </Show>
      </Show>
    </section>
  );
}

function readableError(reason: unknown) {
  return reason instanceof Error ? reason.message : "Something went wrong. Please try again.";
}
