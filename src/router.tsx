import { createRouter } from "@solidjs/router";
import {
  DiscoverPage,
  HomePage,
  NotFoundPage,
  ProfilePage,
} from "./pages";
import { EditorPage, SettingsPage } from "./editor";
import { LinkToolsPage } from "./link-tools";
import { SignInPage } from "./signin";

export const Router = createRouter({
  routes: [
    { path: "/", component: HomePage },
    { path: "/discover", component: DiscoverPage },
    { path: "/create", component: EditorPage },
    { path: "/settings", component: SettingsPage },
    { path: "/settings/import", component: LinkToolsPage },
    { path: "/signin", component: SignInPage },
    { path: "/:handle", component: ProfilePage },
    { path: "*404", component: NotFoundPage },
  ],
});

export const { paths } = Router;
