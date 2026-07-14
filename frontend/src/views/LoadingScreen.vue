<script setup>
import { MessageCircleMore } from "lucide-vue-next";
</script>

<template>
  <main class="loading-screen">
    <span class="loading-mark"><MessageCircleMore :size="25" /></span>
    <span>加载中...</span>
  </main>
</template>
