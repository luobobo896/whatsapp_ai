<script setup>
import { ref, inject } from "vue";
import { messageForError, post } from "../api";

const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "created"]);
const showToast = inject("showToast");
const name = ref("");
const dailyLimit = ref(30);
const submitting = ref(false);

async function submit() {
  submitting.value = true;
  try {
    await post("/api/accounts", { name: name.value, dailyLimit: Number(dailyLimit.value) }, props.csrfToken);
    showToast({ tone: "success", message: "客服账号已创建" });
    emit("created");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally { submitting.value = false; }
}
</script>

<template>
  <el-dialog model-value title="新建客服账号" width="480px" @close="emit('close')">
    <p style="color:#6b736d;font-size:13px;margin:0 0 16px">创建后连接状态为待连接</p>
    <el-input v-model="name" placeholder="例如：售前客服" style="margin-bottom:14px" />
    <el-input-number v-model="dailyLimit" :min="0" :max="10000" placeholder="每日回复上限" style="width:100%" />
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">创建账号</el-button>
    </template>
  </el-dialog>
</template>
