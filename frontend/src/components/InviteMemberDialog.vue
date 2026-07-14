<script setup>
import { ref, inject } from "vue";
import { messageForError, post } from "../api";

const ROLE_LABELS = { owner: "所有者", admin: "管理员", agent: "客服", viewer: "只读成员" };
const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "invited"]);
const showToast = inject("showToast");
const email = ref("");
const role = ref("agent");
const submitting = ref(false);

async function submit() {
  submitting.value = true;
  try {
    const result = await post("/api/members/invitations", { email: email.value, role: role.value }, props.csrfToken);
    showToast({ tone: "success", message: "邀请已创建" });
    emit("invited", result);
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally { submitting.value = false; }
}
</script>

<template>
  <el-dialog model-value title="邀请成员" width="480px" @close="emit('close')">
    <p style="color:#6b736d;font-size:13px;margin:0 0 16px">邀请链接将在 7 天后失效</p>
    <el-input v-model="email" type="email" placeholder="成员邮箱" style="margin-bottom:14px" />
    <el-select v-model="role" style="width:100%">
      <el-option v-for="(label, value) in ROLE_LABELS" :key="value" :value="value" :label="label" />
    </el-select>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">生成邀请</el-button>
    </template>
  </el-dialog>
</template>
