import type { ParentProps } from "solid-js";
import * as stylex from "@stylexjs/stylex";
import { Router } from "./router";
import { styles } from "./styles.stylex";

function Layout(props: ParentProps) {
  return (
    <div {...stylex.attrs(styles.app)}>
      <header {...stylex.attrs(styles.header)}>
        <a {...stylex.attrs(styles.brand)} href="/" aria-label="Vutame home">
          <span {...stylex.attrs(styles.brandMark)}>vu</span>
          <span>vutame</span>
        </a>
        <nav {...stylex.attrs(styles.nav)} aria-label="Primary navigation">
          <a {...stylex.attrs(styles.navLink)} href="/discover">Discover</a>
          <a {...stylex.attrs(styles.navLink)} href="/feed">Feed</a>
          <a {...stylex.attrs(styles.navLink)} href="/create">Dashboard</a>
          <a {...stylex.attrs(styles.navLink)} href="/analytics">Analytics</a>
          <a {...stylex.attrs(styles.navLink)} href="/growth">Growth</a>
          <a {...stylex.attrs(styles.navLink)} href="/operations">Operations</a>
          <a {...stylex.attrs(styles.navLink)} href="/settings/atproto">AT Protocol</a>
          <a {...stylex.attrs(styles.navLink)} href="/settings/import">Share & import</a>
          <a {...stylex.attrs(styles.navLink)} href="/settings/safety">Safety</a>
          <a {...stylex.attrs(styles.navLink)} href="/settings">Settings</a>
          <a {...stylex.attrs(styles.navLink)} href="/signin">Sign in</a>
        </nav>
        <a {...stylex.attrs(styles.primaryButton, styles.smallButton)} href="/create">Claim your name</a>
      </header>
      <main>{props.children}</main>
      <footer {...stylex.attrs(styles.footer)}>
        <span>vutame.com</span>
        <span>Profiles live at vuta.me/@you</span>
      </footer>
    </div>
  );
}

export default function App() {
  return <Router>{(props) => <Layout>{props.children}</Layout>}</Router>;
}
