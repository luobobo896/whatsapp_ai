<script setup>
import { ref, inject, watch } from "vue";
import { ElMessageBox } from "element-plus";
import { messageForError, patch, del } from "../api";

const props = defineProps({
  base: { type: Object, required: true },
  csrfToken: { type: String, required: true },
});
const emit = defineEmits(["close", "updated", "deleted"]);
const showToast = inject("showToast");

const name = ref("");
const description = ref("");
const status = ref("active");
const submitting = ref(false);

watch(() => props.base, (b) => {
  if (b) {
    name.value = b.name || "";
    description.value = b.description || "";
    status.value = b.status || "active";
  }
}, { immediate: true });

async function save() {
  submitting.value = true;
  try {
    await patch(`/api/knowledge/bases/${props.base.id}`, {
      name: name.value, description: description.value, status: status.value,
    }, props.csrfToken);
    showToast({ tone: "success", message: "知识库已更新" });
    emit("updated");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}

async function remove() {
  try {
    await ElMessageBox.confirm("确定要删除这个知识库吗？所有文章也会被删除。", "确认删除", {
      confirmButtonText: "删除", cancelButtonText: "取消", type: "warning",
    });
  } catch { return; }
  submitting.value = true;
  try {
    await del(`/api/knowledge/bases/${props.base.id}`, props.csrfToken);
    showToast({ tone: "success", message: "知识库已删除" });
    emit("deleted");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value title="编辑知识库" width="480px" @close="emit('close')">
    <el-input v-model="name" placeholder="知识库名称" size="large" style="margin-bottom:14px" />
    <el-input v-model="description" placeholder="说明" style="margin-bottom:14px" />
    <el-select v-model="status" style="width:100%">
      <el-option value="active" label="启用" />
      <el-option value="inactive" label="停用" />
    </el-select>
    <template #footer>
      <el-button type="danger" plain :loading="submitting" @click="remove" style="margin-right:auto">删除知识库</el-button>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>
