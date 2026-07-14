<script setup>
import { computed } from "vue";
import { BookOpen, Headphones, MessagesSquare, Server, Users } from "lucide-vue-next";

const props = defineProps({
  health: Object, activeTenant: Object, tenants: Array,
  accounts: Array, knowledgeBases: Array, conversations: Array,
  platformRole: String,
});
const emit = defineEmits(["navigate"]);

const ROLE_LABELS = { owner: "所有者", admin: "管理员", agent: "客服", viewer: "只读成员" };

const metrics = computed(() => {
  const openCount = props.conversations.filter((c) => c.status === "open").length;
  return [
    { label: "服务状态", value: props.health?.status === "ok" ? "正常" : "异常", note: "Go API", icon: Server, tone: props.health?.status === "ok" ? "success" : "error" },
    { label: "客服账号", value: props.activeTenant ? String(props.accounts.length) : "-", note: props.activeTenant?.name || "未选择工作区", icon: Headphones, tone: "info" },
    { label: "知识库", value: props.activeTenant ? String(props.knowledgeBases.length) : "-", note: props.activeTenant ? "当前租户" : "未选择工作区", icon: BookOpen, tone: "muted" },
    { label: "待处理会话", value: props.activeTenant ? String(openCount) : "-", note: props.activeTenant ? "当前租户" : "未选择工作区", icon: MessagesSquare, tone: openCount ? "warning" : "success" },
  ];
});
</script>

<template>
  <div>
    <div class="metric-grid">
      <el-card v-for="m in metrics" :key="m.label" class="metric-card" shadow="never">
        <span :class="['metric-icon', `metric-icon-${m.tone}`]"><component :is="m.icon" :size="20" /></span>
        <div>
          <div class="metric-label">{{ m.label }}</div>
          <div class="metric-value">{{ m.value }}</div>
          <div class="metric-note">{{ m.note }}</div>
        </div>
      </el-card>
    </div>

    <el-row :gutter="20">
      <el-col :span="10">
        <el-card shadow="never">
          <template #header>
            <span style="font-weight:600">当前访问范围</span>
            <div style="font-size:11px;color:#6b736d;margin-top:2px">登录会话中的平台与租户上下文</div>
          </template>
          <el-descriptions :column="1" size="small" border>
            <el-descriptions-item label="平台权限">
              <el-tag :type="platformRole ? 'success' : 'info'" size="small">
                {{ platformRole ? "平台管理员" : "普通成员" }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="当前租户">{{ activeTenant ? activeTenant.name : "未选择" }}</el-descriptions-item>
            <el-descriptions-item label="租户角色">{{ activeTenant?.role ? ROLE_LABELS[activeTenant.role] : "-" }}</el-descriptions-item>
            <el-descriptions-item label="租户状态">
              <el-tag v-if="activeTenant" :type="activeTenant.status === 'active' ? 'success' : 'warning'" size="small">
                {{ activeTenant.status === "active" ? "运行中" : "已暂停" }}
              </el-tag>
              <span v-else>-</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
      <el-col :span="14">
        <el-card shadow="never">
          <template #header>
            <span style="font-weight:600">运营入口</span>
            <div style="font-size:11px;color:#6b736d;margin-top:2px">
              {{ activeTenant ? activeTenant.name : `${tenants.length} 个可访问租户` }}
            </div>
          </template>
          <el-row :gutter="12">
            <el-col v-for="entry in [
              { id: 'accounts', label: '客服账号', desc: 'WhatsApp 服务账号', icon: Headphones },
              { id: 'knowledge', label: '知识库', desc: '客服知识内容', icon: BookOpen },
              { id: 'conversations', label: '会话', desc: '客户沟通记录', icon: MessagesSquare },
              { id: 'members', label: '成员管理', desc: '查看角色与成员状态', icon: Users },
            ]" :key="entry.id" :span="12" style="margin-bottom:12px">
              <el-button
                style="width:100%;height:auto;padding:14px;text-align:left;justify-content:flex-start"
                :disabled="!activeTenant"
                @click="emit('navigate', entry.id)"
              >
                <component :is="entry.icon" :size="18" style="margin-right:10px;flex-shrink:0" />
                <div>
                  <div style="font-weight:600;font-size:13px">{{ entry.label }}</div>
                  <div style="font-size:11px;color:#6b736d;margin-top:2px">{{ entry.desc }}</div>
                </div>
              </el-button>
            </el-col>
          </el-row>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>
