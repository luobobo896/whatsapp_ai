<script setup>
import { BookOpen, Plus } from "lucide-vue-next";
import { formatDate } from "../utils";

defineProps({ bases: Array, canManage: Boolean });
const emit = defineEmits(["create"]);
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
    <el-empty v-if="!bases.length" description="暂无知识库">
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="emit('create')">新建知识库</el-button>
    </el-empty>
    <el-table v-else :data="bases" stripe>
      <el-table-column prop="name" label="知识库" />
      <el-table-column prop="description" label="说明">
        <template #default="{ row }">{{ row.description || "-" }}</template>
      </el-table-column>
      <el-table-column prop="status" label="状态">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">
            {{ row.status === "active" ? "启用" : "停用" }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间">
        <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>
