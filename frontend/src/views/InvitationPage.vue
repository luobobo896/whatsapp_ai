<script setup>
import { ref, reactive } from "vue";
import { useRoute, useRouter } from "vue-router";
import { UserPlus } from "lucide-vue-next";
import { get, messageForError, post } from "../api";
import { setSession } from "../composables/useSession";
import Brand from "../components/Brand.vue";

const route = useRoute();
const router = useRouter();
const formRef = ref(null);
const form = reactive({ email: "", displayName: "", password: "" });
const submitting = ref(false);
const error = ref("");

const rules = {
  email: [{ required: true, message: "请输入邮箱", trigger: "blur" }],
  displayName: [{ required: true, message: "请输入显示名称", trigger: "blur" }],
  password: [{ required: true, min: 12, message: "密码至少12个字符", trigger: "blur" }],
};

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false);
  if (!valid) return;
  submitting.value = true;
  error.value = "";
  try {
    const token = route.params.token;
    const accepted = await post(`/api/invitations/${encodeURIComponent(token)}/accept`, form);
    await post("/api/auth/select-tenant", { tenantId: accepted.tenantId }, accepted.csrfToken);
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
  <main class="invitation-layout">
    <header><Brand compact /></header>
    <div class="invitation-form">
      <div style="text-align:center;margin-bottom:20px">
        <span class="page-icon" style="display:inline-grid;width:44px;height:44px;place-items:center;border-radius:8px;background:#e1f5f0;color:#128c7e;margin-bottom:12px">
          <UserPlus :size="24" />
        </span>
        <h1 style="margin:0 0 8px;font-size:24px">接受成员邀请</h1>
        <p style="margin:0;color:#6b736d;font-size:14px">完成账号信息后进入所属租户工作区</p>
      </div>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" style="margin-bottom:16px" />
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="handleSubmit">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="受邀邮箱" prop="email">
              <el-input v-model="form.email" type="email" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="显示名称" prop="displayName">
              <el-input v-model="form.displayName" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="设置密码" prop="password">
          <el-input v-model="form.password" type="password" autocomplete="new-password" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" native-type="submit" style="width:100%">
            {{ submitting ? "正在加入..." : "确认并进入" }}
          </el-button>
        </el-form-item>
      </el-form>
    </div>
  </main>
</template>
