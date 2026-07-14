<script setup>
import { ref, inject } from "vue";
import { messageForError, post } from "../api";

const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "created"]);
const showToast = inject("showToast");
const name = ref("");
const submitting = ref(false);

async function submit() {
  submitting.value = true;
  try {
    const result = await post("/api/platform/tenants", { name: name.value }, props.csrfToken);
    showToast({ tone: "success", message: "租户已创建" });
    emit("created", result);
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally { submitting.value = false; }
}
</script>

<template>
  <el-dialog model-value title="新建租户" width="480px" @close="emit('close')">
    <p style="color:#6b736d;font-size:13px;margin:0 0 16px">系统将自动生成管理员登录账号和密码</p>
    <el-input v-model="name" placeholder="例如：华东客服中心" size="large" @keyup.enter="submit" />
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">
        {{ submitting ? "创建中..." : "创建租户" }}
      </el-button>
    </template>
  </el-dialog>
</template>
