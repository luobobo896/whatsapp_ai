<script setup>
import { ref, onMounted, provide } from "vue";
import { useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { APIError, get } from "./api";
import { setSession } from "./composables/useSession";
import LoadingScreen from "./views/LoadingScreen.vue";

const router = useRouter();
const loading = ref(true);

provide("showToast", (opts) => ElMessage({ message: opts.message, type: opts.tone === "error" ? "error" : opts.tone === "success" ? "success" : "info" }));

function invitationTokenFromPath() {
  const match = window.location.pathname.match(/^\/invitations\/([^/]+)\/accept\/?$/);
  return match ? decodeURIComponent(match[1]) : "";
}

onMounted(async () => {
  if (invitationTokenFromPath()) { loading.value = false; return; }
  try {
    const result = await get("/api/auth/me");
    setSession(result);
  } catch (error) {
    if (error instanceof APIError && error.status === 401) {
      setSession(null);
      if (router.currentRoute.value.path !== "/login") router.replace("/login");
    }
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <router-view v-if="!loading" />
  <LoadingScreen v-else />
</template>
