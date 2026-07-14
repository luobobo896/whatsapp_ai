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
const attributes = ref("");
const status = ref("active");
const submitting = ref(false);

watch(() => props.article, (a) => {
  if (a) {
    title.value = a.title || "";
    content.value = a.content || "";
    category.value = a.category || "";
    attributes.value = a.attributes || "";
    status.value = a.status || "active";
  }
}, { immediate: true });

function buildAttrs() {
  const val = attributes.value.trim();
  if (!val) return "{}";
  // Accept either JSON or key:value per line format
  if (val.startsWith("{")) return val;
  const lines = val.split("\n").filter(Boolean);
  const obj = {};
  for (const line of lines) {
    const idx = line.indexOf(":");
    if (idx > 0) obj[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
  }
  return JSON.stringify(obj);
}

async function submit() {
  if (!title.value.trim()) return;
  submitting.value = true;
  try {
    const body = {
      title: title.value, content: content.value,
      category: category.value, attributes: buildAttrs(),
      status: status.value,
    };
    if (isEdit) {
      await patch(`/api/knowledge/bases/${props.baseId}/articles/${props.article.id}`, body, props.csrfToken);
    } else {
      await post(`/api/knowledge/bases/${props.baseId}/articles`, body, props.csrfToken);
    }
    showToast({ tone: "success", message: isEdit ? "已更新" : "已创建" });
    emit("saved");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value :title="isEdit ? '编辑知识条目' : '新建知识条目'" width="600px" @close="emit('close')">
    <el-input v-model="title" placeholder="条目名称（如：羽绒服、螺丝M6）" size="large" style="margin-bottom:14px" />
    <el-input v-model="category" placeholder="分类（如：冬装、五金件）" style="margin-bottom:14px" />
    <el-input v-model="content" type="textarea" :rows="5" placeholder="描述内容" style="margin-bottom:14px" />
    <el-input
      v-model="attributes"
      type="textarea"
      :rows="4"
      placeholder="属性（每行一个，格式：品牌: XXX&#10;材质: 纯棉&#10;尺码: M/L/XL&#10;价格: 99）"
      style="margin-bottom:14px"
    />
    <el-select v-if="isEdit" v-model="status" style="width:100%">
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
