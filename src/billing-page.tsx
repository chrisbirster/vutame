import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchAuthSession, type AuthSession } from "./api";
import { fetchBillingState, openBillingPortal, startCheckout, type BillingState } from "./billing-api";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;

const featureNames: Record<string, string> = {
  custom_domains: "Custom domains",
  advanced_analytics: "90-day analytics",
  premium_themes: "Premium themes",
  branding_removal: "Branding removal",
  api_integrations: "Advanced integrations",
};

export function BillingPage() {
  const [session, setSession] = createSignal<AuthSession>();
  const [state, setState] = createSignal<BillingState>();
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  void bootstrap();

  async function bootstrap() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (!current.authenticated) return;
      setState(await fetchBillingState());
      const params = new URLSearchParams(window.location.search);
      if (params.get("checkout") === "success") setMessage("Checkout completed. Your plan updates after Stripe confirms the subscription webhook.");
      if (params.get("checkout") === "cancel") setMessage("Checkout canceled. Your current plan was not changed.");
    } catch (reason) {
      setError(readable(reason));
    }
  }

  async function upgrade() {
    await run(async () => {
      const url = await startCheckout();
      window.location.assign(url);
    });
  }

  async function manage() {
    await run(async () => {
      const url = await openBillingPortal();
      window.location.assign(url);
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
          <div {...sx(styles.eyebrow)}>BILLING</div>
          <h1 {...sx(styles.title)}>Keep identity free. Pay for operator tools.</h1>
        </div>
        <p {...sx(styles.intro)}>
          Your Vuta, links, social graph, safety controls, exports, and AT Protocol portability stay free. Pro adds operational creator features such as custom domains and longer analytics history.
        </p>
      </div>

      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <Show when={message()}><div {...sx(styles.notice, styles.success)}>{message()}</div></Show>

      <Show when={session() !== undefined} fallback={<div {...sx(styles.panel)}>Loading billing…</div>}>
        <Show when={session()?.authenticated} fallback={<div {...sx(styles.panel)}>Sign in to view your plan and billing settings.</div>}>
          <Show when={state()} fallback={<div {...sx(styles.panel)}>Loading your plan…</div>}>
            {(current) => (
              <div {...sx(styles.grid)}>
                <section {...sx(styles.panel, styles.wide)}>
                  <div {...sx(styles.eyebrow)}>CURRENT PLAN</div>
                  <h2 {...sx(styles.panelTitle)}>{current().plan === "pro" ? "Vutame Pro" : "Vutame Free"}</h2>
                  <Show when={current().subscription}>
                    {(subscription) => (
                      <div {...sx(styles.form)}>
                        <div {...sx(styles.muted)}>Subscription · {subscription().status}</div>
                        <Show when={subscription().current_period_end}>
                          <div {...sx(styles.muted)}>Current period ends · {new Date(subscription().current_period_end || "").toLocaleString()}</div>
                        </Show>
                        <Show when={subscription().cancel_at_period_end}>
                          <div {...sx(styles.muted)}>Cancellation is scheduled for the end of the current period.</div>
                        </Show>
                      </div>
                    )}
                  </Show>
                  <Show
                    when={current().billing_configured}
                    fallback={<p {...sx(styles.help)}>Paid upgrades are not configured on this deployment. Candidate Pro features remain available so self-hosted/local use is not degraded.</p>}
                  >
                    <div {...sx(styles.actions)}>
                      <Show when={current().plan === "pro"} fallback={<button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void upgrade()}>Upgrade with Stripe</button>}>
                        <button {...sx(styles.button)} type="button" disabled={busy()} onClick={() => void manage()}>Manage subscription</button>
                      </Show>
                    </div>
                  </Show>
                </section>

                <section {...sx(styles.panel)}>
                  <h2 {...sx(styles.panelTitle)}>Included now</h2>
                  <Show when={current().entitlements.length > 0} fallback={<p {...sx(styles.help)}>Core Vutame identity features remain available on Free.</p>}>
                    <div {...sx(styles.form)}>
                      <For each={current().entitlements}>{(feature) => <div {...sx(styles.contact)}><strong>{featureNames[feature] || feature}</strong></div>}</For>
                    </div>
                  </Show>
                </section>

                <section {...sx(styles.panel)}>
                  <h2 {...sx(styles.panelTitle)}>Always free</h2>
                  <p {...sx(styles.help)}>Public profile and links, follow/social safety controls, creator data export, basic analytics, and AT Protocol identity/portability are not paywalled.</p>
                </section>
              </div>
            )}
          </Show>
        </Show>
      </Show>
    </section>
  );
}

function readable(reason: unknown) {
  return reason instanceof Error ? reason.message : "Something went wrong.";
}
