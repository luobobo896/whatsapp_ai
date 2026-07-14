<script setup>
import { ref, inject } from "vue";
import { UserPlus } from "lucide-vue-next";
import { messageForError, patch } from "../api";

const ROLE_LABELS = { owner: "所有者", admin: "管理员", agent: "客服", viewer: "只读成员" };
const props = defineProps({ members: Array, canManage: Boolean, csrfToken: String });
const emit = defineEmits(["invite", "changed"]);
const showToast = inject("showToast");
const pendingId = ref("");

async function updateMember(m, changes) {
  if (pendingId.value) return;
  pendingId.value = m.userId;
  try {
    await patch(`/api/members/${m.userId}`, { role: changes.role || m.role, status: changes.status || m.status }, props.csrfToken);
    showToast({ tone: "success", message: "成员信息已更新" });
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
          <span style="font-weight:600">租户成员</span>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">成员角色和访问状态由服务端权限控制</div>
        </div>
        <el-button v-if="canManage" type="primary" :icon="UserPlus" @click="emit('invite')">邀请成员</el-button>
      </div>
    </template>
    <el-empty v-if="!members.length" description="暂无成员">
      <el-button v-if="canManage" type="primary" :icon="UserPlus" @click="emit('invite')">邀请成员</el-button>
    </el-empty>
    <el-table v-else :data="members" stripe>
      <el-table-column label="成员">
        <template #default="{ row }">
          <div style="font-weight:600">{{ row.displayName }}</div>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">{{ row.email }}</div>
        </template>
      </el-table-column>
      <el-table-column label="角色" width="140">
        <template #default="{ row }">
          <el-select
            v-if="canManage && row.role !== 'owner'"
            :model-value="row.role"
            size="small"
            :disabled="pendingId === row.userId"
            @change="(v) => updateMember(row, { role: v })"
          >
            <el-option v-for="(label, value) in ROLE_LABELS" :key="value" :value="value" :label="label" />
          </el-select>
          <span v-else>{{ ROLE_LABELS[row.role] }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">
            {{ row.status === "active" ? "正常" : "已停用" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="加入时间" width="140">
        <template #default="{ row }">{{ new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(new Date(row.createdAt)) }}</template>
      </el-table-column>
      <el-table-column v-if="canManage" label="操作" width="100">
        <template #default="{ row }">
          <el-button
            v-if="row.role !== 'owner'"
            size="small"
            :disabled="pendingId === row.userId"
            @click="updateMember(row, { status: row.status === 'active' ? 'disabled' : 'active' })"
          >
            {{ pendingId === row.userId ? "处理中..." : row.status === "active" ? "停用" : "启用" }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
