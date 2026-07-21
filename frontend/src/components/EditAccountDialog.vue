<script setup>
import { ref, inject, onMounted } from "vue";
import { get, messageForError, patch } from "../api";
import KnowledgeBaseBindingField from "./KnowledgeBaseBindingField.vue";

const props = defineProps({ account: Object, csrfToken: String });
const emit = defineEmits(["close", "updated"]);
const showToast = inject("showToast");
const name = ref(props.account.name);
const dailyLimit = ref(props.account.dailyLimit || 30);
const replyLimit = ref(props.account.replyLimit || 30);
const kbId = ref(Array.isArray(props.account.kbId) ? [...props.account.kbId] : []);
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
    const body = {
      name: name.value,
      dailyLimit: Number(dailyLimit.value),
      replyLimit: Number(replyLimit.value),
      kbId: kbId.value,
    };
    await patch(`/api/accounts/${props.account.id}`, body, props.csrfToken);
    showToast({ tone: "success", message: "账号已更新" });
    emit("updated");
  } catch (e) {
    showToast({ tone: "error", message: messageForError(e) });
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <el-dialog model-value title="编辑客服账号" width="min(680px, calc(100vw - 28px))" class="account-dialog" @close="emit('close')">
    <el-form label-position="top" class="account-form">
      <el-form-item label="账号名称">
        <el-input v-model="name" placeholder="账号名称" />
      </el-form-item>
      <el-form-item>
        <KnowledgeBaseBindingField v-model="kbId" :knowledge-bases="knowledgeBases" />
      </el-form-item>
      <div class="account-limits-grid">
        <el-form-item label="每日回复上限">
          <el-input-number v-model="dailyLimit" :min="0" :max="10000" controls-position="right" />
        </el-form-item>
        <el-form-item label="单次拉取消息数">
          <el-input-number v-model="replyLimit" :min="1" :max="500" controls-position="right" />
          <div class="account-limit-hint">每次从 WhatsApp 拉取的历史消息条数，与「每日回复上限」相互独立</div>
        </el-form-item>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.account-limit-hint {
  margin: 4px 0 0;
  font-size: 11px;
  line-height: 1.45;
  color: var(--app-muted);
}
</style>
