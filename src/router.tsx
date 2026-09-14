import { createRouter } from "@solidjs/router";
import {
  FollowersPage,
  FollowingPage,
  HomePage,
  NotFoundPage,
  ProfilePage,
} from "./pages";
import { AnalyticsPage } from "./analytics-page";
import { DiscoverPage } from "./discover-page";
import { EditorPage, SettingsPage } from "./editor";
import { FeedPage } from "./feed";
import { LinkToolsPage } from "./link-tools";
import { SafetySettingsPage } from "./safety-settings";
import { SignInPage } from "./signin";
import { SocialSettingsPage } from "./social-settings";

export const Router = createRouter({
  routes: [
    { path: "/", component: HomePage },
    { path: "/discover", component: DiscoverPage },
    { path: "/feed", component: FeedPage },
    { path: "/analytics", component: AnalyticsPage },
    { path: "/create", component: EditorPage },
    { path: "/settings", component: SettingsPage },
    { path: "/settings/import", component: LinkToolsPage },
    { path: "/settings/discovery", component: SocialSettingsPage },
    { path: "/settings/safety", component: SafetySettingsPage },
    { path: "/network/:handle/followers", component: FollowersPage },
    { path: "/network/:handle/following", component: FollowingPage },
    { path: "/signin", component: SignInPage },
    { path: "/:handle", component: ProfilePage },
    { path: "*404", component: NotFoundPage },
  ],
});

export const { paths } = Router;
