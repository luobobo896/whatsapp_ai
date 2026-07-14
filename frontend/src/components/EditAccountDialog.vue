<script setup>
import { ref, inject, onMounted } from "vue";
import { get, messageForError, patch } from "../api";

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
  <el-dialog model-value title="编辑客服账号" width="480px" @close="emit('close')">
    <el-input v-model="name" placeholder="账号名称" style="margin-bottom:14px" />
    <div style="margin-bottom:14px">
      <label style="display:block;font-size:13px;color:#3d403d;margin-bottom:4px">绑定知识库</label>
      <el-select v-model="kbId" placeholder="选择知识库" clearable multiple style="width:100%">
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
      <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>
