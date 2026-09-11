import { createRouter } from "@solidjs/router";
import {
  CreatePage,
  DiscoverPage,
  HomePage,
  NotFoundPage,
  ProfilePage,
} from "./pages";
import { SignInPage } from "./signin";

export const Router = createRouter({
  routes: [
    { path: "/", component: HomePage },
    { path: "/discover", component: DiscoverPage },
    { path: "/create", component: CreatePage },
    { path: "/signin", component: SignInPage },
    { path: "/:handle", component: ProfilePage },
    { path: "*404", component: NotFoundPage },
  ],
});

export const { paths } = Router;
