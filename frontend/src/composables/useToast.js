import { ref } from "vue";

const toast = ref(null);
let toastTimer = null;

export function useToast() {
  function showToast(t) {
    toast.value = t;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast.value = null;
    }, 3600);
  }

  return { toast, showToast };
}
