import { createSignal, For } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import type { Profile, ProfileUpdate } from "./api";
import { PROFILE_THEMES, ProfileSurface, normalizeProfileTheme, type ProfileTheme } from "./profile-surface";
import { profileEditorPanelStyles as styles } from "./profile-editor-panel.stylex";

const sx = stylex.attrs;

export function ProfileEditorPanel(props: {
  profile: Profile;
  busy: boolean;
  onSave: (input: ProfileUpdate) => Promise<void>;
}) {
  const [displayName, setDisplayName] = createSignal(props.profile.display_name);
  const [bio, setBio] = createSignal(props.profile.bio);
  const [avatarURL, setAvatarURL] = createSignal(props.profile.avatar_url ?? "");
  const [theme, setTheme] = createSignal<ProfileTheme>(normalizeProfileTheme(props.profile.theme));

  const preview = (): Profile => ({
    ...props.profile,
    display_name: displayName(),
    bio: bio(),
    avatar_url: avatarURL(),
    theme: theme(),
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    await props.onSave({
      display_name: displayName(),
      bio: bio(),
      avatar_url: avatarURL(),
      theme: theme(),
    });
  }

  return (
    <div {...sx(styles.workspace)}>
      <form {...sx(styles.panel)} onSubmit={(event) => void submit(event)}>
        <h2 {...sx(styles.title)}>Profile & theme</h2>
        <p {...sx(styles.copy)}>Edit your public identity and watch the mobile preview update before you save.</p>

        <label {...sx(styles.field)}>
          <span {...sx(styles.label)}>DISPLAY NAME</span>
          <input
            {...sx(styles.input)}
            name="display_name"
            value={displayName()}
            maxlength="80"
            onInput={(event) => setDisplayName(event.currentTarget.value)}
          />
        </label>
        <label {...sx(styles.field)}>
          <span {...sx(styles.label)}>BIO</span>
          <textarea
            {...sx(styles.input, styles.textarea)}
            name="bio"
            maxlength="320"
            value={bio()}
            onInput={(event) => setBio(event.currentTarget.value)}
          />
        </label>
        <label {...sx(styles.field)}>
          <span {...sx(styles.label)}>AVATAR URL</span>
          <input
            {...sx(styles.input)}
            name="avatar_url"
            type="url"
            value={avatarURL()}
            placeholder="https://…"
            onInput={(event) => setAvatarURL(event.currentTarget.value)}
          />
        </label>

        <div {...sx(styles.label)}>THEME</div>
        <div {...sx(styles.themes)} role="radiogroup" aria-label="Profile theme">
          <For each={PROFILE_THEMES}>
            {(option) => (
              <button
                {...sx(styles.theme, theme() === option.id && styles.themeSelected)}
                type="button"
                role="radio"
                aria-checked={theme() === option.id}
                onClick={() => setTheme(option.id)}
              >
                <span {...sx(styles.themeName)}>{option.name}</span>
                <span {...sx(styles.themeDescription)}>{option.description}</span>
              </button>
            )}
          </For>
        </div>

        <button {...sx(styles.button)} type="submit" disabled={props.busy}>
          {props.busy ? "Saving…" : "Save profile"}
        </button>
      </form>

      <aside {...sx(styles.previewColumn)} aria-label="Live mobile profile preview">
        <div {...sx(styles.previewLabel)}>LIVE MOBILE PREVIEW</div>
        <ProfileSurface profile={preview()} preview />
      </aside>
    </div>
  );
}
