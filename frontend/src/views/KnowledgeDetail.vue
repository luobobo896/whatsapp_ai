<script setup>
import { ref, onMounted, inject } from "vue";
import { ArrowLeft, Edit, Plus, Trash2 } from "lucide-vue-next";
import { ElMessageBox } from "element-plus";
import { get, del, messageForError } from "../api";
import { formatDate } from "../utils";
import ArticleDialog from "../components/ArticleDialog.vue";

const props = defineProps({
  base: { type: Object, required: true },
  canManage: { type: Boolean, default: false },
  csrfToken: { type: String, default: "" },
});
const emit = defineEmits(["back", "base-updated", "articles-changed"]);
const showToast = inject("showToast");

const articles = ref([]);
const loading = ref(true);
const articleDialogOpen = ref(false);
const editingArticle = ref(null);

onMounted(async () => {
  await loadArticles();
});

async function loadArticles() {
  loading.value = true;
  try {
    const resp = await get(`/api/knowledge/bases/${props.base.id}/articles`);
    articles.value = resp.articles || [];
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingArticle.value = null;
  articleDialogOpen.value = true;
}

function openEdit(article) {
  editingArticle.value = article;
  articleDialogOpen.value = true;
}

async function removeArticle(article) {
  try {
    await ElMessageBox.confirm(`确定删除"${article.title}"吗？`, "确认删除", {
      confirmButtonText: "删除", cancelButtonText: "取消", type: "warning",
    });
  } catch { return; }
  try {
    await del(`/api/knowledge/bases/${props.base.id}/articles/${article.id}`, props.csrfToken);
    showToast({ tone: "success", message: "知识条目已删除" });
    await loadArticles();
    emit("articles-changed");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  }
}

function onArticleSaved() {
  articleDialogOpen.value = false;
  editingArticle.value = null;
  loadArticles();
  emit("articles-changed");
}
</script>

<template>
  <div class="view-stack">
    <!-- Header: back + base info -->
    <el-card shadow="never">
      <div style="display:flex;align-items:flex-start;justify-content:space-between">
        <div>
          <el-button text :icon="ArrowLeft" @click="emit('back')" style="margin-bottom:8px">返回知识库列表</el-button>
          <h2 style="margin:0;font-size:22px">{{ base.name }}</h2>
          <p style="margin:4px 0 0;color:#6b736d;font-size:13px">{{ base.description || "暂无说明" }}</p>
          <div style="margin-top:8px;display:flex;gap:8px;align-items:center">
            <el-tag :type="base.status === 'active' ? 'success' : 'warning'" size="small">
              {{ base.status === "active" ? "启用" : "停用" }}
            </el-tag>
            <span style="font-size:11px;color:#949e96">创建于 {{ formatDate(base.createdAt) }}</span>
          </div>
        </div>
        <div v-if="canManage" style="display:flex;gap:8px">
          <el-button :icon="Edit" @click="emit('base-updated')">编辑</el-button>
        </div>
      </div>
    </el-card>

    <!-- Articles -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex;align-items:center;justify-content:space-between">
          <span style="font-weight:600">知识条目列表 ({{ articles.length }})</span>
          <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建知识条目</el-button>
        </div>
      </template>
      <el-empty v-if="!loading && !articles.length" description="暂无知识条目，点击上方按钮添加" />
      <el-table v-else v-loading="loading" :data="articles" stripe>
        <el-table-column prop="title" label="标题" show-overflow-tooltip />
        <el-table-column prop="category" label="分类" width="140">
          <template #default="{ row }">{{ row.category || "-" }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">
              {{ row.status === "active" ? "启用" : "停用" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }">{{ formatDate(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column v-if="canManage" label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button text size="small" type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button text size="small" type="danger" :icon="Trash2" @click="removeArticle(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <ArticleDialog
      v-if="articleDialogOpen"
      :base-id="base.id"
      :article="editingArticle"
      :csrf-token="csrfToken"
      @close="articleDialogOpen = false"
      @saved="onArticleSaved"
    />
  </div>
</template>
