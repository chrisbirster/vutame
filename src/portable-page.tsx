import { createSignal, Show } from "solid-js";
import { useParams } from "@solidjs/router";
import * as stylex from "@stylexjs/stylex";
import type { Profile } from "./api";
import { fetchPortableProfile, type PortableProfileResponse } from "./atproto-api";
import { normalizeLinkKind } from "./link-kinds";
import { ProfileSurface } from "./profile-surface";
import { growthStyles as styles } from "./growth.stylex";

const sx = stylex.attrs;

export function PortableProfilePage() {
  const params = useParams();
  const [data, setData] = createSignal<PortableProfileResponse>();
  const [error, setError] = createSignal("");

  void load();

  async function load() {
    const did = params.did ?? "";
    if (!did) {
      setError("portable profile unavailable");
      return;
    }
    try {
      setData(await fetchPortableProfile(did));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "portable profile unavailable");
    }
  }

  const profile = (): Profile | undefined => {
    const source = data()?.profile;
    if (!source) return undefined;
    return {
      id: source.did,
      handle: source.handle || data()?.identity?.handle || source.did,
      display_name: source.display_name || source.handle || data()?.identity?.handle || source.did,
      bio: source.bio || "",
      avatar_url: source.avatar_url,
      theme: source.theme || "midnight",
      verified: source.verified,
      atproto_did: source.did,
      links: source.links.map((link) => ({
        id: link.rkey,
        label: link.label,
        url: link.url,
        kind: normalizeLinkKind(link.kind),
        thumbnail_url: link.thumbnail_url,
        featured: link.featured,
        position: link.position,
        is_active: true,
      })),
    };
  };

  return (
    <section {...sx(styles.page)}>
      <Show when={error()}><div {...sx(styles.notice, styles.error)} role="alert">{error()}</div></Show>
      <Show when={profile()} fallback={!error() ? <div {...sx(styles.panel)}>Loading portable Vuta…</div> : null}>
        {(item) => (
          <>
            <div {...sx(styles.heading)}>
              <div>
                <div {...sx(styles.eyebrow)}>PORTABLE VUTA · AT PROTOCOL</div>
                <h1 {...sx(styles.title)}>{data()?.identity?.handle ? `@${data()?.identity?.handle}` : item().atproto_did}</h1>
              </div>
              <p {...sx(styles.intro)}>
                This profile is rendered from portable <code>com.vutame.*</code> records indexed from AT Protocol. The DID is authoritative; the AT handle shown here is resolved from the DID document rather than trusted from the profile record.
              </p>
            </div>
            <ProfileSurface
              profile={item()}
              showContact={false}
              linkHref={(link) => link.url}
            />
            <div {...sx(styles.panel)}>
              <strong>Network identity</strong>
              <div {...sx(styles.muted)}>{item().atproto_did}</div>
              <Show when={data()?.identity?.handle}><div {...sx(styles.muted)}>AT handle · @{data()?.identity?.handle}</div></Show>
              <div {...sx(styles.muted)}>Indexed · {new Date(data()?.profile.indexed_at || "").toLocaleString()}</div>
            </div>
          </>
        )}
      </Show>
    </section>
  );
}
