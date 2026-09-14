import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAuthSession, type AuthSession } from "./api";
import { growthStyles as styles } from "./growth.stylex";
import {
  addDomain,
  createToken,
  createWebhook,
  deleteDomain,
  deleteWebhook,
  exportURL,
  fetchDomains,
  fetchTokens,
  fetchVerificationRequests,
  fetchWebhooks,
  revokeToken,
  testWebhook,
  verifyDomain,
  type APIToken,
  type CustomDomain,
  type VerificationRequest,
  type Webhook,
} from "./operations-api";

const sx = stylex.attrs;

export function OperationsPage() {
  const [session, setSession] = createSignal<AuthSession>();
  const [domains, setDomains] = createSignal<CustomDomain[]>([]);
  const [tokens, setTokens] = createSignal<APIToken[]>([]);
  const [webhooks, setWebhooks] = createSignal<Webhook[]>([]);
  const [verification, setVerification] = createSignal<VerificationRequest[]>([]);
  const [domainName, setDomainName] = createSignal("");
  const [tokenName, setTokenName] = createSignal("");
  const [tokenScopes, setTokenScopes] = createSignal<string[]>(["profile:read"]);
  const [tokenDays, setTokenDays] = createSignal(90);
  const [webhookURL, setWebhookURL] = createSignal("");
  const [webhookEvents, setWebhookEvents] = createSignal<string[]>(["profile.updated"]);
  const [oneTimeSecret, setOneTimeSecret] = createSignal("");
  const [oneTimeLabel, setOneTimeLabel] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) return;
      const [domainItems, tokenItems, webhookItems, verificationItems] = await Promise.all([
        fetchDomains(), fetchTokens(), fetchWebhooks(), fetchVerificationRequests(),
      ]);
      setDomains(domainItems);
      setTokens(tokenItems);
      setWebhooks(webhookItems);
      setVerification(verificationItems);
    } catch (reason) {
      setError(readable(reason));
    }
  }

  async function submitDomain(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const item = await addDomain(domainName());
      setDomains((items) => [item, ...items]);
      setDomainName("");
      setMessage("Domain added. Publish the TXT record shown below, then verify it.");
    });
  }

  async function verify(item: CustomDomain) {
    await run(async () => {
      const updated = await verifyDomain(item.id);
      setDomains((items) => items.map((candidate) => candidate.id === item.id ? updated : candidate));
      setVerification(await fetchVerificationRequests());
      setMessage(`${updated.hostname} is verified and now canonical for your Vuta.`);
    });
  }

  async function removeDomain(item: CustomDomain) {
    await run(async () => {
      await deleteDomain(item.id);
      setDomains((items) => items.filter((candidate) => candidate.id !== item.id));
    });
  }

  async function submitToken(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const item = await createToken({ name: tokenName(), scopes: tokenScopes(), expires_in_days: tokenDays() });
      setTokens((items) => [item, ...items]);
      setOneTimeLabel("API token — copy it now. Vutame stores only its HMAC hash.");
      setOneTimeSecret(item.token);
      setTokenName("");
    });
  }

  async function revoke(item: APIToken) {
    await run(async () => {
      await revokeToken(item.id);
      setTokens((items) => items.filter((candidate) => candidate.id !== item.id));
    });
  }

  async function submitWebhook(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const item = await createWebhook({ url: webhookURL(), events: webhookEvents() });
      setWebhooks((items) => [item, ...items]);
      setOneTimeLabel("Webhook signing secret — copy it now and verify X-Vutame-Signature on every delivery.");
      setOneTimeSecret(item.signing_secret);
      setWebhookURL("");
    });
  }

  async function run(action: () => Promise<void>) {
    setBusy(true); setError(""); setMessage("");
    try { await action(); } catch (reason) { setError(readable(reason)); } finally { setBusy(false); }
  }

  function toggleScope(scope: string, checked: boolean) {
    setTokenScopes((items) => checked ? Array.from(new Set([...items, scope])) : items.filter((item) => item !== scope));
  }

  function toggleEvent(value: string, checked: boolean) {
    setWebhookEvents((items) => checked ? Array.from(new Set([...items, value])) : items.filter((item) => item !== value));
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.heading)}>
        <div><div {...sx(styles.eyebrow)}>CREATOR OPERATIONS</div><h1 {...sx(styles.title)}>Own the edges.</h1></div>
        <p {...sx(styles.intro)}>Bring your own domain, export your data, and connect Vutame to automation through scoped tokens and signed webhooks.</p>
      </div>
      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <Show when={message()}><div {...sx(styles.notice, styles.success)}>{message()}</div></Show>
      <Show when={oneTimeSecret()}>
        <section {...sx(styles.panel, styles.wide)}>
          <strong>{oneTimeLabel()}</strong>
          <code {...sx(styles.generated)}>{oneTimeSecret()}</code>
          <button {...sx(styles.button)} type="button" onClick={() => void navigator.clipboard.writeText(oneTimeSecret())}>Copy secret</button>
        </section>
      </Show>
      <Show when={session() !== undefined} fallback={<div {...sx(styles.panel)}>Loading creator operations…</div>}>
        <Show when={session()?.authenticated} fallback={<div {...sx(styles.panel)}>Sign in to manage creator operations.</div>}>
          <div {...sx(styles.grid)}>
            <section {...sx(styles.panel, styles.wide)}>
              <h2 {...sx(styles.panelTitle)}>Custom domains + verification</h2>
              <p {...sx(styles.help)}>Add a hostname, publish the generated TXT record, then verify. The first verified domain becomes your canonical public URL and proves control for the Vutame verified badge.</p>
              <form {...sx(styles.row)} onSubmit={submitDomain}>
                <label {...sx(styles.field)}><span {...sx(styles.label)}>DOMAIN</span><input {...sx(styles.input)} value={domainName()} onInput={(event) => setDomainName(event.currentTarget.value)} placeholder="me.example.com" /></label>
                <div {...sx(styles.actions)}><button {...sx(styles.button)} type="submit" disabled={busy() || !domainName().trim()}>Add domain</button></div>
              </form>
              <div {...sx(styles.contacts)}>
                <For each={domains()}>{(item) => (
                  <div {...sx(styles.contact)}>
                    <div><strong>{item.hostname}</strong><div {...sx(styles.muted)}>{item.verified_at ? "VERIFIED" : "DNS VERIFICATION PENDING"}</div></div>
                    <Show when={!item.verified_at}><code>{item.dns_name} TXT {item.dns_value}</code></Show>
                    <div {...sx(styles.actions)}>
                      <Show when={!item.verified_at}><button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void verify(item)}>Verify DNS</button></Show>
                      <button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void removeDomain(item)}>Remove</button>
                    </div>
                  </div>
                )}</For>
              </div>
              <Show when={verification().length > 0}><p {...sx(styles.help)}>Verification history: {verification()[0]?.status} via {verification()[0]?.method}.</p></Show>
            </section>

            <section {...sx(styles.panel)}>
              <h2 {...sx(styles.panelTitle)}>Scoped API tokens</h2>
              <p {...sx(styles.help)}>Tokens are shown once and stored as HMAC hashes. Use only the scopes your integration needs.</p>
              <form {...sx(styles.form)} onSubmit={submitToken}>
                <label {...sx(styles.field)}><span {...sx(styles.label)}>NAME</span><input {...sx(styles.input)} value={tokenName()} onInput={(event) => setTokenName(event.currentTarget.value)} placeholder="My automation" /></label>
                <For each={["profile:read", "profile:write", "analytics:read", "contacts:read"]}>{(scope) => <label {...sx(styles.checkRow)}><input {...sx(styles.checkbox)} type="checkbox" checked={tokenScopes().includes(scope)} onChange={(event) => toggleScope(scope, event.currentTarget.checked)} /><span>{scope}</span></label>}</For>
                <label {...sx(styles.field)}><span {...sx(styles.label)}>EXPIRES</span><select {...sx(styles.input)} value={String(tokenDays())} onChange={(event) => setTokenDays(Number(event.currentTarget.value))}><option value="30">30 days</option><option value="90">90 days</option><option value="365">365 days</option><option value="0">No expiry</option></select></label>
                <button {...sx(styles.button)} type="submit" disabled={busy() || !tokenName().trim() || tokenScopes().length === 0}>Create token</button>
              </form>
              <div {...sx(styles.contacts)}><For each={tokens()}>{(item) => <div {...sx(styles.contact)}><div><strong>{item.name}</strong><div {...sx(styles.muted)}>{item.prefix}… · {item.scopes.join(", ")}</div></div><button {...sx(styles.button)} type="button" onClick={() => void revoke(item)}>Revoke</button></div>}</For></div>
            </section>

            <section {...sx(styles.panel)}>
              <h2 {...sx(styles.panelTitle)}>Signed webhooks</h2>
              <p {...sx(styles.help)}>HTTPS-only public endpoints receive durable, retried POST deliveries signed with HMAC-SHA256.</p>
              <form {...sx(styles.form)} onSubmit={submitWebhook}>
                <label {...sx(styles.field)}><span {...sx(styles.label)}>ENDPOINT</span><input {...sx(styles.input)} value={webhookURL()} onInput={(event) => setWebhookURL(event.currentTarget.value)} placeholder="https://example.com/hooks/vutame" /></label>
                <For each={["profile.updated", "link.featured"]}>{(eventName) => <label {...sx(styles.checkRow)}><input {...sx(styles.checkbox)} type="checkbox" checked={webhookEvents().includes(eventName)} onChange={(event) => toggleEvent(eventName, event.currentTarget.checked)} /><span>{eventName}</span></label>}</For>
                <button {...sx(styles.button)} type="submit" disabled={busy() || !webhookURL().trim() || webhookEvents().length === 0}>Add webhook</button>
              </form>
              <div {...sx(styles.contacts)}><For each={webhooks()}>{(item) => <div {...sx(styles.contact)}><div><strong>{item.url}</strong><div {...sx(styles.muted)}>{item.events.join(", ")}</div></div><div {...sx(styles.actions)}><button {...sx(styles.button)} type="button" onClick={() => void run(async () => { await testWebhook(item.id); setMessage("Test delivery queued."); })}>Test</button><button {...sx(styles.button)} type="button" onClick={() => void run(async () => { await deleteWebhook(item.id); setWebhooks((items) => items.filter((candidate) => candidate.id !== item.id)); })}>Remove</button></div></div>}</For></div>
            </section>

            <section {...sx(styles.panel, styles.wide)}>
              <h2 {...sx(styles.panelTitle)}>Portable data export</h2>
              <p {...sx(styles.help)}>Download your account, profile, links, creator metadata, analytics events, contacts, domains, verification history, token metadata, and webhook configuration as JSON. Visitor hashes and token hashes are intentionally excluded.</p>
              <a {...sx(styles.button)} href={exportURL()}>Download JSON export</a>
            </section>
          </div>
        </Show>
      </Show>
    </section>
  );
}

function readable(reason: unknown) { return reason instanceof Error ? reason.message : "Something went wrong."; }
