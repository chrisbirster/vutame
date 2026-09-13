import { For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import type { Link, Profile } from "./api";
import { linkKindMeta } from "./link-kinds";
import { profileSurfaceStyles as styles } from "./profile-surface.stylex";

const sx = stylex.attrs;

export const PROFILE_THEMES = [
  { id: "midnight", name: "Midnight", description: "Dark, soft, and Vutame-native." },
  { id: "paper", name: "Paper", description: "Warm editorial serif with crisp cards." },
  { id: "neon", name: "Neon", description: "High-contrast creator terminal energy." },
  { id: "forest", name: "Forest", description: "Organic greens with pill-shaped links." },
] as const;

export type ProfileTheme = (typeof PROFILE_THEMES)[number]["id"];

export function normalizeProfileTheme(theme: string | undefined): ProfileTheme {
  return PROFILE_THEMES.some((candidate) => candidate.id === theme)
    ? (theme as ProfileTheme)
    : "midnight";
}

export function ProfileSurface(props: { profile: Profile; preview?: boolean }) {
  const theme = () => themeStyles(normalizeProfileTheme(props.profile.theme));
  const links = () => props.profile.links
    .filter((link) => link.is_active && linkVisibleNow(link))
    .slice()
    .sort((left, right) => {
      if (left.featured !== right.featured) return left.featured ? -1 : 1;
      if (left.position !== right.position) return left.position - right.position;
      return left.id.localeCompare(right.id);
    });
  const initial = () => (props.profile.display_name || props.profile.handle || "V").trim().slice(0, 1).toUpperCase();

  return (
    <article {...sx(styles.frame, props.preview && styles.previewFrame, theme().surface)}>
      <Show
        when={props.profile.avatar_url}
        fallback={<div {...sx(styles.avatar, props.preview && styles.previewAvatar, theme().avatar)} aria-hidden="true">{initial()}</div>}
      >
        {(avatarURL) => (
          <img
            {...sx(styles.avatar, props.preview && styles.previewAvatar, theme().avatar)}
            src={avatarURL()}
            alt=""
            width="94"
            height="94"
            loading={props.preview ? "eager" : "lazy"}
            decoding="async"
          />
        )}
      </Show>
      <h1 {...sx(styles.title, props.preview && styles.previewTitle)}>{props.profile.display_name || `@${props.profile.handle}`}</h1>
      <div {...sx(styles.handle)}>@{props.profile.handle}</div>
      <Show when={props.profile.bio}>
        <p {...sx(styles.bio, props.preview && styles.previewBio, theme().bio)}>{props.profile.bio}</p>
      </Show>
      <Show
        when={links().length > 0}
        fallback={<div {...sx(styles.empty, theme().bio)}>No public links yet.</div>}
      >
        <div {...sx(styles.stack, props.preview && styles.previewStack)}>
          <For each={links()}>
            {(link) => {
              const meta = () => linkKindMeta(link.kind);
              return (
                <a
                  {...sx(styles.link, link.featured && styles.featuredLink, props.preview && styles.previewLink, theme().link)}
                  href={props.preview ? undefined : link.url}
                  target={props.preview ? undefined : "_blank"}
                  rel={props.preview ? undefined : "noreferrer"}
                  aria-disabled={props.preview ? "true" : undefined}
                  aria-label={`${link.label} — ${meta().label}${link.featured ? ", featured" : ""}`}
                >
                  <span {...sx(styles.linkLead)}>
                    <Show
                      when={link.thumbnail_url}
                      fallback={<span {...sx(styles.kindBadge, props.preview && styles.previewKindBadge)} aria-hidden="true">{meta().badge}</span>}
                    >
                      {(thumbnailURL) => (
                        <img
                          {...sx(styles.thumbnail, props.preview && styles.previewThumbnail)}
                          src={thumbnailURL()}
                          alt=""
                          width="48"
                          height="48"
                          loading={props.preview ? "eager" : "lazy"}
                          decoding="async"
                        />
                      )}
                    </Show>
                    <span {...sx(styles.linkCopy)}>
                      <span {...sx(styles.linkLabel)}>{link.label}</span>
                      <span {...sx(styles.linkMeta)}>
                        <span {...sx(styles.linkKind, theme().bio)}>{meta().label}</span>
                        <Show when={link.featured}><span {...sx(styles.featuredBadge)}>FEATURED</span></Show>
                      </span>
                    </span>
                  </span>
                  <span {...sx(styles.arrow)} aria-hidden="true">↗</span>
                </a>
              );
            }}
          </For>
        </div>
      </Show>
      <a {...sx(styles.badge, theme().bio)} href={props.preview ? undefined : "/"} aria-disabled={props.preview ? "true" : undefined}>
        made on vutame
      </a>
    </article>
  );
}

function linkVisibleNow(link: Link) {
  const now = Date.now();
  if (link.visible_from) {
    const from = new Date(link.visible_from).getTime();
    if (!Number.isFinite(from) || now < from) return false;
  }
  if (link.visible_until) {
    const until = new Date(link.visible_until).getTime();
    if (!Number.isFinite(until) || now >= until) return false;
  }
  return true;
}

function themeStyles(theme: ProfileTheme) {
  switch (theme) {
    case "paper":
      return { surface: styles.paperSurface, bio: styles.paperBio, link: styles.paperLink, avatar: styles.paperAvatar };
    case "neon":
      return { surface: styles.neonSurface, bio: styles.neonBio, link: styles.neonLink, avatar: styles.neonAvatar };
    case "forest":
      return { surface: styles.forestSurface, bio: styles.forestBio, link: styles.forestLink, avatar: styles.forestAvatar };
    default:
      return { surface: styles.midnightSurface, bio: styles.midnightBio, link: styles.midnightLink, avatar: styles.midnightAvatar };
  }
}
