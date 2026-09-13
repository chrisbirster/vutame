import { createSignal, For, Show } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import {
  deleteOwnedAvatar,
  uploadOwnedAvatar,
  type Profile,
  type ProfileUpdate,
} from "./api";
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
  const [mediaBusy, setMediaBusy] = createSignal(false);
  const [mediaMessage, setMediaMessage] = createSignal("");
  const [mediaError, setMediaError] = createSignal("");

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

  async function uploadAvatar(event: Event & { currentTarget: HTMLInputElement }) {
    const input = event.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    setMediaBusy(true);
    setMediaMessage("");
    setMediaError("");
    try {
      const asset = await uploadOwnedAvatar(file);
      setAvatarURL(asset.url);
      setMediaMessage("Avatar uploaded and saved.");
    } catch (reason) {
      setMediaError(reason instanceof Error ? reason.message : "Avatar upload failed.");
    } finally {
      input.value = "";
      setMediaBusy(false);
    }
  }

  async function removeAvatar() {
    setMediaBusy(true);
    setMediaMessage("");
    setMediaError("");
    try {
      await deleteOwnedAvatar();
      setAvatarURL("");
      setMediaMessage("Avatar removed.");
    } catch (reason) {
      setMediaError(reason instanceof Error ? reason.message : "Avatar delete failed.");
    } finally {
      setMediaBusy(false);
    }
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

        <div {...sx(styles.mediaBox)}>
          <div>
            <div {...sx(styles.label)}>MANAGED AVATAR</div>
            <p {...sx(styles.mediaHelp)}>Upload JPEG, PNG, WebP, or GIF up to 5 MB. Vutame stores and serves the current avatar for this profile.</p>
          </div>
          <div {...sx(styles.mediaActions)}>
            <label {...sx(styles.uploadButton, (props.busy || mediaBusy()) && styles.disabledButton)}>
              {mediaBusy() ? "Working…" : "Upload image"}
              <input
                {...sx(styles.fileInput)}
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                disabled={props.busy || mediaBusy()}
                onChange={(event) => void uploadAvatar(event)}
              />
            </label>
            <button
              {...sx(styles.secondaryButton)}
              type="button"
              disabled={props.busy || mediaBusy() || !avatarURL()}
              onClick={() => void removeAvatar()}
            >
              Remove avatar
            </button>
          </div>
          <Show when={mediaMessage()}><div {...sx(styles.mediaMessage)}>{mediaMessage()}</div></Show>
          <Show when={mediaError()}><div {...sx(styles.mediaError)}>{mediaError()}</div></Show>
        </div>

        <label {...sx(styles.field)}>
          <span {...sx(styles.label)}>EXTERNAL AVATAR URL <span {...sx(styles.optional)}>OPTIONAL</span></span>
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
                aria-checked={theme() === option.id ? "true" : "false"}
                onClick={() => setTheme(option.id)}
              >
                <span {...sx(styles.themeName)}>{option.name}</span>
                <span {...sx(styles.themeDescription)}>{option.description}</span>
              </button>
            )}
          </For>
        </div>

        <button {...sx(styles.button)} type="submit" disabled={props.busy || mediaBusy()}>
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
