<script setup>
import { ref, computed, inject } from "vue";
import { Edit, MessageCircle, Plus, Plug, QrCode, Trash2 } from "lucide-vue-next";
import { ElMessageBox } from "element-plus";
import { del, get, messageForError } from "../api";
import { formatDate } from "../utils";
import QrLoginCard from "../components/QrLoginCard.vue";

const props = defineProps({ accounts: Array, canManage: Boolean, csrfToken: String, knowledgeBases: Array });
const emit = defineEmits(["create", "chat", "edit", "changed"]);
const qrAccount = ref(null);
const deletingAccountId = ref("");
const showToast = inject("showToast");

const kbMap = computed(() => {
  const map = {};
  (props.knowledgeBases || []).forEach((kb) => { map[kb.id] = kb.name; });
  return map;
});

function knowledgePreview(ids) {
  return (ids || []).slice(0, 2).map((id) => kbMap.value[id] || id.slice(0, 8));
}

function remainingKnowledgeCount(ids) {
  return Math.max((ids || []).length - 2, 0);
}

function onAccountChanged() {
  qrAccount.value = null;
  emit("changed");
}

async function removeAccount(account) {
  try {
    await ElMessageBox.confirm(
      `确定删除“${account.name}”吗？该客服的全部会话记录将永久删除，知识库不会受影响。`,
      "确认删除",
      { confirmButtonText: "删除", cancelButtonText: "取消", type: "warning" },
    );
  } catch { return; }
  deletingAccountId.value = account.id;
  try {
    const result = await del(`/api/accounts/${account.id}`, props.csrfToken);
    if (result?.status === "deleting") {
      showToast({ tone: "info", message: "客服账号正在后台删除" });
      const deleted = await waitForAccountDeletion(account.id);
      showToast({
        tone: deleted ? "success" : "info",
        message: deleted ? "客服账号已删除" : "删除仍在后台进行，请稍后刷新",
      });
    } else {
      showToast({ tone: "success", message: "客服账号已删除" });
    }
    emit("changed");
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
    emit("changed");
  } finally {
    deletingAccountId.value = "";
  }
}

async function waitForAccountDeletion(accountId) {
  const deadline = Date.now() + 40000;
  while (Date.now() < deadline) {
    try {
      const response = await get("/api/accounts");
      if (!(response.accounts || []).some((item) => item.id === accountId)) return true;
    } catch {
      // A later poll may succeed after a transient network or gateway restart.
    }
    await new Promise((resolve) => setTimeout(resolve, Math.min(1000, deadline - Date.now())));
  }
  return false;
}
</script>

<template>
  <el-card shadow="never" class="accounts-panel">
    <template #header>
      <div class="panel-header">
        <div>
          <span class="panel-title">客服账号</span>
          <div class="panel-subtitle">管理 WhatsApp 连接、回复容量与知识库范围</div>
        </div>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建账号</el-button>
      </div>
    </template>
    <el-empty v-if="!accounts.length" description="暂无客服账号">
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建账号</el-button>
    </el-empty>
    <div v-else class="accounts-table-wrap">
    <el-table :data="accounts" class="accounts-table">
      <el-table-column prop="name" label="账号名称" min-width="144">
        <template #default="{ row }">
          <div class="account-name-cell">
            <strong>{{ row.name }}</strong>
            <code>{{ row.accountKey }}</code>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="连接状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'connected' ? 'success' : row.status === 'disabled' ? 'warning' : 'info'" size="small">
            {{ row.status === "connected" ? "已连接" : row.status === "disabled" ? "已停用" : "待连接" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="知识库范围" min-width="230">
        <template #default="{ row }">
          <div v-if="row.kbId && row.kbId.length" class="knowledge-summary-cell">
            <strong>{{ row.kbId.length }} 个知识库</strong>
            <span>{{ knowledgePreview(row.kbId).join("、") }}</span>
            <span v-if="remainingKnowledgeCount(row.kbId)" class="knowledge-summary-more">另 {{ remainingKnowledgeCount(row.kbId) }} 个</span>
          </div>
          <span v-else class="empty-value">未绑定</span>
        </template>
      </el-table-column>
      <el-table-column label="今日回复" width="126">
        <template #default="{ row }">
          <div class="reply-capacity">
            <strong>{{ row.dailyReplies || 0 }}</strong>
            <span>/ {{ row.dailyLimit || "不限" }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="162">
        <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="300">
        <template #default="{ row }">
          <el-button
            v-if="canManage"
            size="small"
            :icon="Edit"
            @click="emit('edit', row)"
          >
            编辑
          </el-button>
          <el-button
            v-if="canManage && row.status === 'pending'"
            size="small"
            type="primary"
            :icon="QrCode"
            @click="qrAccount = row"
          >
            扫码登录
          </el-button>
          <el-button
            v-if="canManage && row.status === 'connected'"
            size="small"
            type="success"
            :icon="MessageCircle"
            @click="emit('chat', row)"
          >
            进入聊天
          </el-button>
          <el-button
            v-if="canManage && row.status === 'connected'"
            size="small"
            :icon="Plug"
            @click="qrAccount = row"
          >
            管理连接
          </el-button>
          <el-button
            v-if="canManage"
            text
            size="small"
            type="danger"
            :icon="Trash2"
            :loading="deletingAccountId === row.id"
            aria-label="删除客服账号"
            title="删除客服账号"
            @click="removeAccount(row)"
          />
        </template>
      </el-table-column>
    </el-table>
    </div>
  </el-card>

  <!-- QR Login Dialog -->
  <el-dialog
    v-if="qrAccount"
    model-value
    title="WhatsApp 扫码登录"
    width="500px"
    @close="qrAccount = null"
  >
    <QrLoginCard
      :account="qrAccount"
      :csrf-token="csrfToken"
      @close="qrAccount = null"
      @connected="onAccountChanged"
      @disconnected="onAccountChanged"
    />
  </el-dialog>
</template>
