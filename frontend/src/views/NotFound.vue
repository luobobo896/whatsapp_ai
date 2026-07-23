<script setup>
import { useRouter } from "vue-router";
import { AlertCircle } from "lucide-vue-next";

const router = useRouter();

const goHome = () => {
  router.push("/");
};
</script>

<template>
  <main class="not-found-page">
    <span class="not-found-icon"><AlertCircle :size="48" /></span>
    <h1>404</h1>
    <p>页面未找到</p>
    <button class="go-home-button" @click="goHome">返回首页</button>
  </main>
</template>

<style scoped>
.not-found-page {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  gap: 1rem;
  padding: 2rem;
}

.not-found-icon {
  color: #128c7e;
}

h1 {
  font-size: 4rem;
  font-weight: 600;
  margin: 0;
  color: #075e54;
}

p {
  font-size: 1.125rem;
  color: #667781;
  margin: 0;
}

.go-home-button {
  padding: 0.625rem 1.5rem;
  background-color: #128c7e;
  color: white;
  border: none;
  border-radius: 0.375rem;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.2s;
}

.go-home-button:hover {
  background-color: #075e54;
}
</style>
