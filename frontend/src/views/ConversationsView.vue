<script setup>
import { computed } from "vue";
import { MessagesSquare } from "lucide-vue-next";
import { formatDate } from "../utils";

const props = defineProps({ conversations: Array, accounts: Array });
const accountNames = computed(() => {
  const map = new Map();
  props.accounts.forEach((a) => map.set(a.id, a.name));
  return map;
});
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <span style="font-weight:600">客户会话</span>
      <div style="font-size:11px;color:#6b736d;margin-top:2px">当前租户的客户沟通记录</div>
    </template>
    <el-empty v-if="!conversations.length" description="暂无会话" />
    <el-table v-else :data="conversations" stripe>
      <el-table-column prop="customer" label="客户" />
      <el-table-column label="客服账号">
        <template #default="{ row }">{{ accountNames.get(row.accountId) || "-" }}</template>
      </el-table-column>
      <el-table-column prop="lastMessage" label="最近消息">
        <template #default="{ row }">{{ row.lastMessage || "-" }}</template>
      </el-table-column>
      <el-table-column prop="status" label="状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'open' ? 'success' : 'info'" size="small">
            {{ row.status === "open" ? "进行中" : "已结束" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间">
        <template #default="{ row }">{{ formatDate(row.lastMessageAt) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
