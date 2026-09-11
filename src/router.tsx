import { createRouter } from "@solidjs/router";
import {
  CreatePage,
  DiscoverPage,
  HomePage,
  NotFoundPage,
  ProfilePage,
} from "./pages";

export const Router = createRouter({
  routes: [
    { path: "/", component: HomePage },
    { path: "/discover", component: DiscoverPage },
    { path: "/create", component: CreatePage },
    { path: "/:handle", component: ProfilePage },
    { path: "*404", component: NotFoundPage },
  ],
});

export const { paths } = Router;
