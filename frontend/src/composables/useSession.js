import { ref, readonly } from "vue";

// Shared session state used across the app.
const session = ref(null);

export function useSession() {
  return { session: readonly(session), setSession };
}

export function setSession(value) {
  session.value = value;
}
