<script setup>
import { ref, inject } from "vue";
import { MessagesSquare } from "lucide-vue-next";
import { get } from "../api";
import { formatDate } from "../utils";

defineProps({ conversations: Array });
const showToast = inject("showToast");

const messagesOpen = ref(false);
const messages = ref([]);
const selectedConv = ref(null);
const loadingMessages = ref(false);

async function openMessages(conv) {
  selectedConv.value = conv;
  messagesOpen.value = true;
  loadingMessages.value = true;
  try {
    const resp = await get(`/api/conversations/${conv.conversationId}/messages`);
    messages.value = (resp.messages || []).reverse();
  } catch (e) {
    showToast({ tone: "error", message: "加载消息失败" });
  } finally {
    loadingMessages.value = false;
  }
}
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <span style="font-weight:600">客户会话</span>
      <div style="font-size:11px;color:#6b736d;margin-top:2px">WhatsApp 客服对话记录</div>
    </template>
    <el-empty v-if="!conversations.length" description="暂无会话，接入 WhatsApp 后自动显示" />
    <el-table v-else :data="conversations" stripe @row-click="openMessages" style="cursor:pointer">
      <el-table-column prop="customerName" label="客户" />
      <el-table-column prop="lastMessage" label="最近消息" show-overflow-tooltip />
      <el-table-column label="消息数" width="80">
        <template #default="{ row }">
          <el-tag size="small" type="info">{{ row.messageCount }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="最后活跃" width="170">
        <template #default="{ row }">{{ row.lastMessageAt ? formatDate(row.lastMessageAt) : "-" }}</template>
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
          textAlign: m.role === 'customer' ? 'left' : 'left',
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
  </el-dialog>
</template>
