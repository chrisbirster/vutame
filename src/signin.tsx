import { createSignal, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  APIError,
  fetchAuthSession,
  logoutAuth,
  requestAuthCode,
  verifyAuthCode,
  type AuthSession,
  type AuthUser,
} from "./api";
import { signInStyles as styles } from "./signin.stylex";

const sx = stylex.attrs;

type Step = "email" | "code" | "account";

export function SignInPage() {
  const [step, setStep] = createSignal<Step>("email");
  const [email, setEmail] = createSignal("");
  const [code, setCode] = createSignal("");
  const [challengeID, setChallengeID] = createSignal("");
  const [session, setSession] = createSignal<AuthSession | undefined>();
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [message, setMessage] = createSignal("");

  const user = (): AuthUser | undefined => {
    const current = session();
    return current?.authenticated ? current.user : undefined;
  };

  void refreshSession();

  async function refreshSession() {
    try {
      const current = await fetchAuthSession();
      setSession(current);
      if (current.authenticated) setStep("account");
    } catch {
      setSession({ authenticated: false });
    }
  }

  async function submitEmail(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const result = await requestAuthCode(email());
      setChallengeID(result.challenge_id);
      setStep("code");
      setMessage(`We sent a six-digit code to ${email().trim()}.`);
    } catch (reason) {
      setError(authError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function submitCode(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await verifyAuthCode(challengeID(), email(), code());
      setSession({ authenticated: true, user: result.user });
      setStep("account");
      setMessage("");
    } catch (reason) {
      setError(authError(reason));
    } finally {
      setBusy(false);
    }
  }

  async function signOut() {
    setBusy(true);
    setError("");
    try {
      await logoutAuth();
      setSession({ authenticated: false });
      setStep("email");
      setCode("");
      setChallengeID("");
      setMessage("Signed out.");
    } catch (reason) {
      setError(authError(reason));
    } finally {
      setBusy(false);
    }
  }

  function startOver() {
    setCode("");
    setChallengeID("");
    setError("");
    setMessage("");
    setStep("email");
  }

  return (
    <section {...sx(styles.page)}>
      <div {...sx(styles.card)}>
        <div {...sx(styles.eyebrow)}>YOUR VUTA ACCOUNT</div>
        <Show
          when={step() === "account" && user()}
          fallback={
            <>
              <h1 {...sx(styles.title)}>{step() === "code" ? "Check your inbox." : "Sign in without a password."}</h1>
              <p {...sx(styles.copy)}>
                {step() === "code"
                  ? "Enter the six-digit code. Codes expire after ten minutes and can only be used once."
                  : "Use your email to create or access your Vutame account. No password to remember."}
              </p>

              <Show when={step() === "email"}>
                <form {...sx(styles.form)} onSubmit={submitEmail}>
                  <label {...sx(styles.label)} for="signin-email">Email address</label>
                  <input
                    {...sx(styles.input)}
                    id="signin-email"
                    type="email"
                    autocomplete="email"
                    required
                    value={email()}
                    onInput={(event) => setEmail(event.currentTarget.value)}
                    placeholder="you@example.com"
                  />
                  <button {...sx(styles.button)} type="submit" disabled={busy()}>
                    {busy() ? "Sending…" : "Send me a code"}
                  </button>
                </form>
              </Show>

              <Show when={step() === "code"}>
                <form {...sx(styles.form)} onSubmit={submitCode}>
                  <label {...sx(styles.label)} for="signin-code">Six-digit code</label>
                  <input
                    {...sx(styles.input, styles.codeInput)}
                    id="signin-code"
                    inputmode="numeric"
                    autocomplete="one-time-code"
                    pattern="[0-9]{6}"
                    maxlength="6"
                    required
                    value={code()}
                    onInput={(event) => setCode(event.currentTarget.value.replace(/\D/g, "").slice(0, 6))}
                    placeholder="000000"
                  />
                  <button {...sx(styles.button)} type="submit" disabled={busy() || code().length !== 6}>
                    {busy() ? "Verifying…" : "Verify and sign in"}
                  </button>
                  <button {...sx(styles.secondaryButton)} type="button" onClick={startOver}>Use a different email</button>
                </form>
              </Show>
            </>
          }
        >
          {(account) => (
            <>
              <h1 {...sx(styles.title)}>You’re signed in.</h1>
              <p {...sx(styles.copy)}>Your session is stored in a secure browser cookie; Vutame never stores the raw session token in the database.</p>
              <div {...sx(styles.account)}>
                <div {...sx(styles.label)}>ACCOUNT</div>
                <div {...sx(styles.accountEmail)}>{account().email}</div>
                <div {...sx(styles.actions)}>
                  <a {...sx(styles.linkAction)} href="/create">Choose your Vuta name</a>
                  <button {...sx(styles.secondaryButton)} type="button" disabled={busy()} onClick={() => void signOut()}>
                    {busy() ? "Signing out…" : "Sign out"}
                  </button>
                </div>
              </div>
            </>
          )}
        </Show>

        <Show when={message()}><div {...sx(styles.message)}>{message()}</div></Show>
        <Show when={error()}><div {...sx(styles.message, styles.error)}>{error()}</div></Show>
      </div>
    </section>
  );
}

function authError(reason: unknown) {
  if (reason instanceof APIError) {
    if (reason.status === 503) return "Email sign-in is not configured in this environment yet.";
    if (reason.status === 401) return "That code is invalid or expired. Request a new code and try again.";
    return reason.message;
  }
  return reason instanceof Error ? reason.message : "Something went wrong. Please try again.";
}
