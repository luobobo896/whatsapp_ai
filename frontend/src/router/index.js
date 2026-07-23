import { createRouter, createWebHistory } from "vue-router";

const routes = [
  {
    path: "/invitations/:token/accept",
    name: "invitation",
    component: () => import("../views/InvitationPage.vue"),
  },
  {
    path: "/login",
    name: "login",
    component: () => import("../views/LoginPage.vue"),
  },
  {
    path: "/",
    name: "dashboard",
    component: () => import("../views/Dashboard.vue"),
  },
  {
    // 404 catch-all route - must be last
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: () => import("../views/NotFound.vue"),
  },
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
