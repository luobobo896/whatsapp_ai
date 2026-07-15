<script setup>
import { computed, inject, ref } from "vue";
import { Trash2 } from "lucide-vue-next";
import { del } from "../api";
import { formatDate } from "../utils";

const props = defineProps({ conversations: Array, accounts: Array, canManage: Boolean, csrfToken: String });
const emit = defineEmits(["chat", "changed"]);
const showToast = inject("showToast");

const deletingConv = ref(null);
const filterAccountId = ref("");

const visibleConversations = computed(() => {
  if (!filterAccountId.value) return props.conversations;
  return props.conversations.filter(c => c.accountId === filterAccountId.value);
});

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
    <el-table v-else :data="visibleConversations" stripe style="cursor:pointer" @row-click="emit('chat', $event)">
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

</template>
