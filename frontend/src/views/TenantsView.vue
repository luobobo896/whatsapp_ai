<script setup>
import { ref, inject } from "vue";
import { Plus } from "lucide-vue-next";
import { messageForError, patch } from "../api";

const ROLE_LABELS = { owner: "所有者", admin: "管理员", agent: "客服", viewer: "只读成员" };
const props = defineProps({ tenants: Array, platformRole: String, activeTenantId: [String, null], csrfToken: String });
const emit = defineEmits(["select", "create", "changed"]);
const showToast = inject("showToast");
const pendingId = ref("");

async function toggleStatus(tenant) {
  if (pendingId.value) return;
  const next = tenant.status === "active" ? "suspended" : "active";
  pendingId.value = tenant.id;
  try {
    await patch(`/api/platform/tenants/${tenant.id}/status`, { status: next, reason: next === "suspended" ? "由平台管理台暂停" : "" }, props.csrfToken);
    showToast({ tone: "success", message: next === "active" ? "租户已恢复" : "租户已暂停" });
    emit("changed");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally { pendingId.value = ""; }
}
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <div>
          <span style="font-weight:600">{{ platformRole ? "平台租户" : "我的工作区" }}</span>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">
            {{ platformRole ? "创建租户并管理服务状态" : "选择当前会话使用的租户" }}
          </div>
        </div>
        <el-button v-if="platformRole" type="primary" :icon="Plus" @click="emit('create')">新建租户</el-button>
      </div>
    </template>
    <el-empty v-if="!tenants.length" description="暂无租户">
      <el-button v-if="platformRole" type="primary" :icon="Plus" @click="emit('create')">新建租户</el-button>
    </el-empty>
    <el-table v-else :data="tenants" stripe>
      <el-table-column label="租户">
        <template #default="{ row }">
          <div style="font-weight:600">{{ row.name }}</div>
          <div v-if="row.id === activeTenantId" style="font-size:11px;color:#6b736d;margin-top:2px">当前工作区</div>
        </template>
      </el-table-column>
      <el-table-column label="状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">
            {{ row.status === "active" ? "运行中" : "已暂停" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="成员身份">
        <template #default="{ row }">
          {{ row.role ? (ROLE_LABELS[row.role] || row.role) : "-" }}
          <div v-if="row.membershipStatus !== 'active'" style="font-size:11px;color:#d94535;margin-top:2px">成员身份已停用</div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button
            v-if="row.membershipStatus === 'active' && row.status === 'active'"
            :type="row.id === activeTenantId ? 'default' : 'primary'"
            size="small"
            :disabled="row.id === activeTenantId || pendingId === row.id"
            @click="emit('select', row.id)"
          >
            {{ row.id === activeTenantId ? "已选择" : pendingId === row.id ? "进入中..." : "进入" }}
          </el-button>
          <el-button
            v-if="platformRole"
            size="small"
            :disabled="pendingId === row.id"
            @click="toggleStatus(row)"
          >
            {{ pendingId === row.id ? "处理中..." : row.status === "active" ? "暂停" : "恢复" }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
