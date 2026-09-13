import { For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import type { Profile } from "./api";
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
  const links = () => props.profile.links.filter((link) => link.is_active);
  const initial = () => (props.profile.display_name || props.profile.handle || "V").trim().slice(0, 1).toUpperCase();

  return (
    <article {...sx(styles.frame, props.preview && styles.previewFrame, theme().surface)}>
      <Show
        when={props.profile.avatar_url}
        fallback={<div {...sx(styles.avatar, props.preview && styles.previewAvatar, theme().avatar)}>{initial()}</div>}
      >
        {(avatarURL) => (
          <img
            {...sx(styles.avatar, props.preview && styles.previewAvatar, theme().avatar)}
            src={avatarURL()}
            alt=""
            loading={props.preview ? "eager" : "lazy"}
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
                  {...sx(styles.link, props.preview && styles.previewLink, theme().link)}
                  href={props.preview ? undefined : link.url}
                  target={props.preview ? undefined : "_blank"}
                  rel={props.preview ? undefined : "noreferrer"}
                  aria-disabled={props.preview ? "true" : undefined}
                >
                  <span {...sx(styles.linkLead)}>
                    <Show
                      when={link.thumbnail_url}
                      fallback={<span {...sx(styles.kindBadge, props.preview && styles.previewKindBadge)}>{meta().badge}</span>}
                    >
                      {(thumbnailURL) => (
                        <img
                          {...sx(styles.thumbnail, props.preview && styles.previewThumbnail)}
                          src={thumbnailURL()}
                          alt=""
                          loading={props.preview ? "eager" : "lazy"}
                        />
                      )}
                    </Show>
                    <span {...sx(styles.linkCopy)}>
                      <span {...sx(styles.linkLabel)}>{link.label}</span>
                      <span {...sx(styles.linkKind, theme().bio)}>{meta().label}</span>
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
