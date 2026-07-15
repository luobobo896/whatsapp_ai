<script setup>
import { ref, inject, onMounted } from "vue";
import { get, messageForError, post } from "../api";
import KnowledgeBaseBindingField from "./KnowledgeBaseBindingField.vue";

const props = defineProps({ csrfToken: String });
const emit = defineEmits(["close", "created"]);
const showToast = inject("showToast");
const name = ref("");
const dailyLimit = ref(30);
const replyLimit = ref(30);
const kbId = ref([]);
const knowledgeBases = ref([]);
const submitting = ref(false);

onMounted(async () => {
  try {
    const resp = await get("/api/knowledge/bases");
    knowledgeBases.value = resp.bases || [];
  } catch (error) {
    showToast({ tone: "error", message: messageForError(error) });
  }
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
  <el-dialog model-value title="新建客服账号" width="min(680px, calc(100vw - 28px))" class="account-dialog" @close="emit('close')">
    <p class="account-dialog-intro">创建后需扫码登录对接 WhatsApp。知识库可在后续随时调整。</p>
    <el-form label-position="top" class="account-form">
      <el-form-item label="账号名称">
        <el-input v-model="name" placeholder="例如：售前客服" />
      </el-form-item>
      <el-form-item>
        <KnowledgeBaseBindingField v-model="kbId" :knowledge-bases="knowledgeBases" />
      </el-form-item>
      <div class="account-limits-grid">
        <el-form-item label="每日回复上限">
          <el-input-number v-model="dailyLimit" :min="0" :max="10000" controls-position="right" />
        </el-form-item>
        <el-form-item label="每次加载消息上限">
          <el-input-number v-model="replyLimit" :min="1" :max="500" controls-position="right" />
        </el-form-item>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">创建账号</el-button>
    </template>
  </el-dialog>
</template>
