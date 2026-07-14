<script setup>
import { inject } from "vue";
import { Edit, Plus, Trash2 } from "lucide-vue-next";
import { ElMessageBox } from "element-plus";
import { del, messageForError, patch } from "../api";
import { formatDate } from "../utils";

const props = defineProps({ bases: Array, canManage: Boolean, csrfToken: String });
const emit = defineEmits(["create", "edit", "detail", "changed"]);
const showToast = inject("showToast");

async function toggleStatus(base) {
  const next = base.status === "active" ? "inactive" : "active";
  try {
    await patch(`/api/knowledge/bases/${base.id}`, { status: next }, props.csrfToken);
    emit("changed");
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  }
}

async function remove(base) {
  try {
    await ElMessageBox.confirm(`确定删除"${base.name}"吗？所有文章也会被删除。`, "确认删除", {
      confirmButtonText: "删除", cancelButtonText: "取消", type: "warning",
    });
  } catch { return; }
  try {
    await del(`/api/knowledge/bases/${base.id}`, props.csrfToken);
    emit("changed");
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  }
}
</script>

<template>
  <el-card shadow="never">
    <template #header>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <div>
          <span style="font-weight:600">知识库</span>
          <div style="font-size:11px;color:#6b736d;margin-top:2px">当前租户可用于客服回复的知识内容</div>
        </div>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建知识库</el-button>
      </div>
    </template>
    <el-empty v-if="!bases.length" description="暂无知识库" />
    <el-table v-else :data="bases" stripe @row-click="(row) => emit('detail', row)" style="cursor:pointer">
      <el-table-column prop="name" label="知识库" />
      <el-table-column prop="description" label="说明">
        <template #default="{ row }">{{ row.description || "-" }}</template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">
            {{ row.status === "active" ? "启用" : "停用" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column v-if="canManage" label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button text size="small" type="primary" :icon="Edit" @click.stop="emit('edit', row)">编辑</el-button>
          <el-button text size="small" @click.stop="toggleStatus(row)">
            {{ row.status === "active" ? "停用" : "启用" }}
          </el-button>
          <el-button text size="small" type="danger" :icon="Trash2" @click.stop="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
