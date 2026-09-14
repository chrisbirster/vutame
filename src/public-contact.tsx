import { createEffect, createSignal, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { fetchPublicContactBlock, submitPublicContact, type ContactBlock } from "./growth-api";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;

export function PublicContactCapture(props: { handle: string }) {
  const [block, setBlock] = createSignal<ContactBlock | null | undefined>(undefined);
  const [email, setEmail] = createSignal("");
  const [consent, setConsent] = createSignal(false);
  const [website, setWebsite] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [message, setMessage] = createSignal("");
  const [error, setError] = createSignal("");

  createEffect(() => props.handle, (handle) => {
    setBlock(undefined);
    setMessage("");
    setError("");
    void fetchPublicContactBlock(handle)
      .then(setBlock)
      .catch((reason) => {
        setError(readableError(reason));
        setBlock(null);
      });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    setError("");
    try {
      const campaign = new URLSearchParams(window.location.search).get("utm_campaign") ?? "";
      await submitPublicContact(props.handle, {
        email: email(),
        consent: consent(),
        campaign,
        website: website(),
      });
      setEmail("");
      setConsent(false);
      setMessage("Thanks — your email was shared with this creator.");
    } catch (reason) {
      setError(readableError(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Show when={block()}>
      {(item) => (
        <section {...sx(styles.contactCapture)} aria-label="Contact creator">
          <h2 {...sx(styles.contactCaptureTitle)}>{item().heading}</h2>
          <Show when={item().description}><p {...sx(styles.help)}>{item().description}</p></Show>
          <Show when={message()}><div {...sx(styles.notice, styles.success)}>{message()}</div></Show>
          <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
          <form {...sx(styles.form)} onSubmit={submit}>
            <label {...sx(styles.field)}>
              <span {...sx(styles.label)}>EMAIL</span>
              <input {...sx(styles.input)} type="email" value={email()} onInput={(event) => setEmail(event.currentTarget.value)} autocomplete="email" required />
            </label>
            <label {...sx(styles.honeypot)} aria-hidden="true">
              Website
              <input tabindex="-1" autocomplete="off" value={website()} onInput={(event) => setWebsite(event.currentTarget.value)} />
            </label>
            <label {...sx(styles.checkRow)}>
              <input {...sx(styles.checkbox)} type="checkbox" checked={consent()} onChange={(event) => setConsent(event.currentTarget.checked)} required />
              <span {...sx(styles.help)}>{item().consent_text}</span>
            </label>
            <button {...sx(styles.button)} type="submit" disabled={busy()}>{busy() ? "Submitting…" : item().button_label}</button>
          </form>
        </section>
      )}
    </Show>
  );
}

function readableError(reason: unknown) {
  return reason instanceof Error ? reason.message : "Contact submission failed. Please try again.";
}
