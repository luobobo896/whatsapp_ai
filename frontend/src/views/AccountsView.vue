<script setup>
import { ref } from "vue";
import { Headphones, MessageCircle, Plus, Plug, QrCode } from "lucide-vue-next";
import { formatDate } from "../utils";
import QrLoginCard from "../components/QrLoginCard.vue";

const props = defineProps({ accounts: Array, canManage: Boolean, csrfToken: String });
const emit = defineEmits(["create", "chat"]);
const qrAccount = ref(null);

function onAccountChanged() {
  qrAccount.value = null;
  emit("changed");
}
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <div>
          <span style="font-weight:600">客服账号</span>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">当前租户的 WhatsApp 服务账号</div>
        </div>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建账号</el-button>
      </div>
    </template>
    <el-empty v-if="!accounts.length" description="暂无客服账号">
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建账号</el-button>
    </el-empty>
    <el-table v-else :data="accounts" stripe>
      <el-table-column prop="name" label="账号名称" />
      <el-table-column prop="accountKey" label="系统标识" />
      <el-table-column prop="status" label="连接状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'connected' ? 'success' : row.status === 'disabled' ? 'warning' : 'info'" size="small">
            {{ row.status === "connected" ? "已连接" : row.status === "disabled" ? "已停用" : "待连接" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="dailyLimit" label="每日上限">
        <template #default="{ row }">{{ row.dailyLimit || "不限" }}</template>
      </el-table-column>
      <el-table-column label="创建时间">
        <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button
            v-if="row.status !== 'connected'"
            size="small"
            type="primary"
            :icon="QrCode"
            @click="qrAccount = row"
          >
            扫码登录
          </el-button>
          <el-button
            v-if="row.status === 'connected'"
            size="small"
            type="success"
            :icon="MessageCircle"
            @click="emit('chat', row)"
          >
            进入聊天
          </el-button>
          <el-button
            v-if="row.status === 'connected'"
            size="small"
            :icon="Plug"
            @click="qrAccount = row"
          >
            管理连接
          </el-button>
        </template>
      </el-table-column>
    </el-table>
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
