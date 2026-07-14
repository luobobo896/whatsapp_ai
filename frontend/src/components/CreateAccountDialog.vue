<script setup>
import { ref, inject, onMounted } from "vue";
import { get, messageForError, post } from "../api";

const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "created"]);
const showToast = inject("showToast");
const name = ref("");
const dailyLimit = ref(30);
const replyLimit = ref(30);
const kbId = ref("");
const knowledgeBases = ref([]);
const submitting = ref(false);

onMounted(async () => {
  try {
    const resp = await get("/api/knowledge/bases");
    knowledgeBases.value = resp.bases || [];
  } catch { /* ignore */ }
});

async function submit() {
  submitting.value = true;
  try {
    await post(
      "/api/accounts",
      {
        name: name.value,
        dailyLimit: Number(dailyLimit.value),
        replyLimit: Number(replyLimit.value),
        kbId: kbId.value,
      },
      props.csrfToken,
    );
    showToast({ tone: "success", message: "客服账号已创建" });
    emit("created");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value title="新建客服账号" width="480px" @close="emit('close')">
    <p style="color:#6b736d;font-size:13px;margin:0 0 16px">创建后需扫码登录对接 WhatsApp</p>
    <el-input v-model="name" placeholder="例如：售前客服" style="margin-bottom:14px" />
    <div style="margin-bottom:14px">
      <label style="display:block;font-size:13px;color:#3d403d;margin-bottom:4px">绑定知识库</label>
      <el-select v-model="kbId" placeholder="选择知识库（可选）" clearable style="width:100%">
        <el-option
          v-for="kb in knowledgeBases"
          :key="kb.id"
          :value="kb.id"
          :label="kb.name"
        />
      </el-select>
    </div>
    <el-input-number v-model="dailyLimit" :min="0" :max="10000" placeholder="每日回复上限" style="width:100%;margin-bottom:14px" />
    <el-input-number v-model="replyLimit" :min="1" :max="500" placeholder="每次加载消息上限" style="width:100%" />
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">创建账号</el-button>
    </template>
  </el-dialog>
</template>
