<script setup>
import { ref, computed, inject } from "vue";
import { MessageCircle, Trash2 } from "lucide-vue-next";
import { del, get } from "../api";
import { formatDate } from "../utils";

const props = defineProps({ conversations: Array, accounts: Array, canManage: Boolean, csrfToken: String });
const emit = defineEmits(["chat", "changed"]);
const showToast = inject("showToast");

const messagesOpen = ref(false);
const messages = ref([]);
const selectedConv = ref(null);
const loadingMessages = ref(false);
const deletingConv = ref(null);
const filterAccountId = ref("");

const visibleConversations = computed(() => {
  if (!filterAccountId.value) return props.conversations;
  return props.conversations.filter(c => c.accountId === filterAccountId.value);
});

async function openMessages(conv) {
  selectedConv.value = conv;
  messagesOpen.value = true;
  loadingMessages.value = true;
  try {
    const resp = await get(`/api/conversations/${conv.conversationId}/messages?accountId=${encodeURIComponent(conv.accountId)}`);
    messages.value = (resp.messages || []).reverse();
  } catch {
    showToast({ tone: "error", message: "加载消息失败" });
  } finally {
    loadingMessages.value = false;
  }
}

async function deleteConversation(conv) {
  deletingConv.value = conv.conversationId;
  try {
    await del(`/api/conversations/${conv.conversationId}?accountId=${encodeURIComponent(conv.accountId)}`, props.csrfToken);
    showToast({ tone: "success", message: "会话已删除" });
    emit("changed");
  } catch {
    showToast({ tone: "error", message: "删除失败" });
  } finally {
    deletingConv.value = null;
  }
}
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <div>
          <span style="font-weight:600">客户会话</span>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">WhatsApp 客服对话记录</div>
        </div>
        <el-select
          v-if="accounts?.length"
          v-model="filterAccountId"
          placeholder="按账号筛选"
          clearable
          size="small"
          style="width:160px"
        >
          <el-option
            v-for="a in accounts"
            :key="a.id"
            :value="a.id"
            :label="a.name"
          />
        </el-select>
      </div>
    </template>
    <el-empty v-if="!conversations.length" description="暂无会话，接入 WhatsApp 后自动显示" />
    <el-table v-else :data="visibleConversations" stripe style="cursor:pointer" @row-click="openMessages">
      <el-table-column prop="customerName" label="客户" min-width="120" />
      <el-table-column prop="lastMessage" label="最近消息" show-overflow-tooltip min-width="200" />
      <el-table-column label="消息数" width="80">
        <template #default="{ row }">
          <el-tag size="small" type="info">{{ row.messageCount }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="最后活跃" width="170">
        <template #default="{ row }">{{ row.lastMessageAt ? formatDate(row.lastMessageAt) : "-" }}</template>
      </el-table-column>
      <el-table-column v-if="canManage" label="操作" width="90" fixed="right">
        <template #default="{ row }">
          <el-popconfirm
            title="确定删除该会话及其所有消息？"
            confirm-button-text="删除"
            cancel-button-text="取消"
            @confirm="deleteConversation(row)"
          >
            <template #reference>
              <el-button
                type="danger"
                :icon="Trash2"
                circle
                size="small"
                :loading="deletingConv === row.conversationId"
                @click.stop
              />
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <!-- Message detail dialog -->
  <el-dialog
    v-model="messagesOpen"
    :title="selectedConv?.customerName || '对话详情'"
    width="640px"
  >
    <div v-loading="loadingMessages" style="max-height:500px;overflow-y:auto">
      <div v-if="!messages.length && !loadingMessages" style="text-align:center;color:#6b736d;padding:40px">
        暂无消息记录
      </div>
      <div
        v-for="m in messages" :key="m.id"
        :style="{
          marginBottom: '14px',
          padding: '10px 14px',
          borderRadius: '8px',
          background: m.role === 'customer' ? '#e1f5f0' : '#f5f7fa',
          textAlign: 'left',
        }"
      >
        <div style="display:flex;justify-content:space-between;margin-bottom:6px">
          <span style="font-size:11px;font-weight:700;color:#128c7e">
            {{ m.role === 'customer' ? '客户' : '客服' }}
          </span>
          <span style="font-size:10px;color:#949e96">{{ formatDate(m.createdAt) }}</span>
        </div>
        <div style="font-size:13px;line-height:1.6;white-space:pre-wrap;word-break:break-word">{{ m.content }}</div>
        <div v-if="m.knowledgeIds && m.knowledgeIds !== '[]'" style="margin-top:6px;font-size:10px;color:#949e96">
          参考知识: {{ m.knowledgeIds }}
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="messagesOpen = false">关闭</el-button>
      <el-button
        v-if="canManage && selectedConv"
        type="primary"
        :icon="MessageCircle"
        @click="emit('chat', selectedConv)"
      >
        继续处理
      </el-button>
    </template>
  </el-dialog>
</template>
