<script setup>
import { inject } from "vue";
import { Check } from "lucide-vue-next";

const props = defineProps({ created: Object });
const emit = defineEmits(["close"]);
const showToast = inject("showToast");
const { tenant, credentials } = props.created;

async function copyAll() {
  await navigator.clipboard.writeText(`租户：${tenant.name}\n账号：${credentials.email}\n密码：${credentials.password}`);
  showToast({ tone: "success", message: "账号密码已复制" });
}
async function copyVal(val, label) {
  await navigator.clipboard.writeText(val);
  showToast({ tone: "success", message: `${label}已复制` });
}
</script>

<template>
  <el-dialog model-value :title="`租户已创建`" width="520px" @close="emit('close')">
    <div style="display:flex;align-items:center;gap:12px;padding:12px;border-radius:8px;background:#e6f7ed;color:#1fa855;margin-bottom:16px">
      <Check :size="22" />
      <p style="margin:0;color:#3d403d;font-size:12px">请立即保存管理员账号和密码，关闭后无法再次查看密码。</p>
    </div>
    <div class="credential-row">
      <span style="font-size:13px;font-weight:600;white-space:nowrap">登录账号</span>
      <el-input :model-value="credentials.email" readonly />
      <el-button @click="copyVal(credentials.email, '账号')">复制</el-button>
    </div>
    <div class="credential-row">
      <span style="font-size:13px;font-weight:600;white-space:nowrap">初始密码</span>
      <el-input :model-value="credentials.password" readonly />
      <el-button @click="copyVal(credentials.password, '密码')">复制</el-button>
    </div>
    <template #footer>
      <el-button @click="copyAll">复制账号密码</el-button>
      <el-button type="primary" @click="emit('close')">完成</el-button>
    </template>
  </el-dialog>
</template>
