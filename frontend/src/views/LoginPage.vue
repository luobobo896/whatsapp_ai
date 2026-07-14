<script setup>
import { ref, reactive } from "vue";
import { useRouter } from "vue-router";
import { ShieldCheck } from "lucide-vue-next";
import { get, messageForError, post } from "../api";
import { setSession } from "../composables/useSession";
import Brand from "../components/Brand.vue";

const router = useRouter();
const formRef = ref(null);
const form = reactive({ email: "", password: "" });
const submitting = ref(false);
const error = ref("");

const rules = {
  email: [{ required: true, message: "请输入邮箱地址", trigger: "blur" }],
  password: [{ required: true, message: "请输入密码", trigger: "blur" }],
};

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false);
  if (!valid) return;
  submitting.value = true;
  error.value = "";
  try {
    await post("/api/auth/login", form);
    const session = await get("/api/auth/me");
    setSession(session);
    router.replace("/");
  } catch (e) {
    error.value = messageForError(e);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <main class="auth-layout">
    <section class="auth-brand-panel">
      <Brand />
      <div class="auth-brand-status">
        <span><ShieldCheck :size="18" /></span>
        <div>
          <strong>安全运营入口</strong>
          <small>会话与租户权限由服务端统一管理</small>
        </div>
      </div>
      <p class="auth-footnote">企业内部管理系统</p>
    </section>
    <section class="auth-form-area">
      <div class="auth-form">
        <div class="auth-form-header">
          <h1>登录管理台</h1>
          <p>使用管理员或租户成员账号继续</p>
        </div>
        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" style="margin-bottom:16px" />
        <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleSubmit">
          <el-form-item prop="email">
            <el-input v-model="form.email" type="email" placeholder="name@company.com" size="large" />
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" placeholder="输入登录密码" show-password size="large" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" size="large" :loading="submitting" native-type="submit" style="width:100%">
              {{ submitting ? "正在登录..." : "登录" }}
            </el-button>
          </el-form-item>
        </el-form>
      </div>
    </section>
  </main>
</template>
