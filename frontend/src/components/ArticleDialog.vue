<script setup>
import { ref, inject, watch } from "vue";
import { messageForError, post, patch } from "../api";

const props = defineProps({
  baseId: { type: String, required: true },
  article: { type: Object, default: null },
  csrfToken: { type: String, required: true },
});
const emit = defineEmits(["close", "saved"]);
const showToast = inject("showToast");

const isEdit = !!props.article;
const title = ref("");
const content = ref("");
const category = ref("");
const status = ref("active");
const submitting = ref(false);

watch(() => props.article, (a) => {
  if (a) {
    title.value = a.title || "";
    content.value = a.content || "";
    category.value = a.category || "";
    status.value = a.status || "active";
  }
}, { immediate: true });

async function submit() {
  if (!title.value.trim()) return;
  submitting.value = true;
  try {
    if (isEdit) {
      await patch(`/api/knowledge/bases/${props.baseId}/articles/${props.article.id}`, {
        title: title.value, content: content.value, category: category.value, status: status.value,
      }, props.csrfToken);
    } else {
      await post(`/api/knowledge/bases/${props.baseId}/articles`, {
        title: title.value, content: content.value, category: category.value,
      }, props.csrfToken);
    }
    showToast({ tone: "success", message: isEdit ? "文章已更新" : "文章已创建" });
    emit("saved");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value :title="isEdit ? '编辑文章' : '新建文章'" width="560px" @close="emit('close')">
    <el-input v-model="title" placeholder="文章标题" size="large" style="margin-bottom:14px" />
    <el-input v-model="category" placeholder="分类（如：售后政策、产品介绍）" style="margin-bottom:14px" />
    <el-input v-model="content" type="textarea" :rows="8" placeholder="文章内容" />
    <el-select v-if="isEdit" v-model="status" style="width:100%;margin-top:14px">
      <el-option value="active" label="启用" />
      <el-option value="inactive" label="停用" />
    </el-select>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" :disabled="!title.trim()" @click="submit">
        {{ isEdit ? "保存" : "创建" }}
      </el-button>
    </template>
  </el-dialog>
</template>
