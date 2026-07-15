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
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
