<script setup>
import { ref, inject } from "vue";
import { messageForError, post } from "../api";

const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "created"]);
const showToast = inject("showToast");
const name = ref("");
const description = ref("");
const submitting = ref(false);

async function submit() {
  submitting.value = true;
  try {
    await post("/api/knowledge/bases", { name: name.value, description: description.value }, props.csrfToken);
    showToast({ tone: "success", message: "知识库已创建" });
    emit("created");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally { submitting.value = false; }
}
</script>

<template>
  <el-dialog model-value title="新建知识库" width="480px" @close="emit('close')">
    <p style="color:#6b736d;font-size:13px;margin:0 0 16px">知识内容按当前租户隔离</p>
    <el-input v-model="name" placeholder="例如：商品与售后政策" style="margin-bottom:14px" />
    <el-input v-model="description" placeholder="说明（可选)" />
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">创建知识库</el-button>
    </template>
  </el-dialog>
</template>
