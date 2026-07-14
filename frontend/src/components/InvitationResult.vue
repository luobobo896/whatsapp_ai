<script setup>
import { inject } from "vue";
import { Check } from "lucide-vue-next";

const props = defineProps({ invitation: Object });
const emit = defineEmits(["close"]);
const showToast = inject("showToast");

const link = `${window.location.origin}/invitations/${encodeURIComponent(props.invitation.token)}/accept`;

async function copy() {
  await navigator.clipboard.writeText(link);
  showToast({ tone: "success", message: "邀请链接已复制" });
}
</script>

<template>
  <el-dialog model-value title="邀请已生成" width="520px" @close="emit('close')">
    <div style="display:flex;align-items:center;gap:12px;padding:12px;border-radius:8px;background:#e6f7ed;color:#1fa855;margin-bottom:16px">
      <Check :size="22" />
      <p style="margin:0;color:#3d403d;font-size:12px">该链接只显示一次，请立即发送给受邀成员。</p>
    </div>
    <div class="credential-row">
      <el-input :model-value="link" readonly aria-label="邀请链接" />
      <el-button type="primary" @click="copy">复制链接</el-button>
    </div>
    <template #footer>
      <el-button type="primary" @click="emit('close')">完成</el-button>
    </template>
  </el-dialog>
</template>
