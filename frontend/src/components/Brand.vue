<script setup>
import { MessageCircleMore } from "lucide-vue-next";

defineProps({
  compact: { type: Boolean, default: false },
});
</script>

<template>
  <div :class="['brand', { 'brand-compact': compact }]">
    <span class="brand-mark"><MessageCircleMore :size="22" :stroke-width="2.2" /></span>
    <span class="brand-copy">
      <strong>WhatsApp AI Ops</strong>
      <small v-if="!compact">智能客服运营台</small>
    </span>
  </div>
</template>
